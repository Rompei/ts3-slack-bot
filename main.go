package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"strconv"

	"github.com/Rompei/inco"
	"github.com/darfk/ts3"
)

// Client is object of client.
type Client struct {
	CliID       int    `json:"cliId"`
	CID         int    `json:"cId"`
	Name        string `json:"name"`
	ChannelName string `json:"channelName"`
	IsNotified  bool   `json:"isNotified"`
}

// NewClient is constructor of Client.
func NewClient(cliID, cID int, name string) *Client {
	return &Client{
		CliID: cliID,
		CID:   cID,
		Name:  name,
	}
}

// NewClientFromMap create Client from map.
func NewClientFromMap(m map[string]string) (*Client, error) {
	cliID, ok := m["clid"]
	if !ok {
		return nil, errors.New("information not enough")
	}
	cliIDInt, err := strconv.Atoi(cliID)
	if err != nil {
		return nil, err
	}
	cid, ok := m["cid"]
	if !ok {
		return nil, errors.New("information not enough")
	}

	cidInt, err := strconv.Atoi(cid)
	if err != nil {
		return nil, err
	}
	name, ok := m["client_nickname"]
	if !ok {
		return nil, errors.New("information not enough")
	}

	return NewClient(cliIDInt, cidInt, name), nil
}

func main() {

	var (
		username   string
		password   string
		serverID   int
		webhookURL string
		output     string
		debug      bool
	)

	flag.StringVar(&username, "u", "", "TS3 server query username")
	flag.StringVar(&password, "p", "", "TS3 server query password")
	flag.IntVar(&serverID, "id", 1, "Server ID")
	flag.StringVar(&webhookURL, "url", "", "WebHookURL")
	flag.StringVar(&output, "o", "clients.json", "Output file")
	flag.BoolVar(&debug, "d", false, "Debug")
	flag.Parse()

	if username == "" || password == "" || webhookURL == "" {
		panic(errors.New("Not enough options"))
	}

	client, err := ts3.NewClient(":10011")
	if err != nil {
		panic(err)
	}
	defer client.Close()

	_, err = client.Exec(ts3.Login(username, password))
	if err != nil {
		panic(err)
	}

	_, err = client.Exec(ts3.Use(serverID))
	if err != nil {
		panic(err)
	}

	// Get client list.
	newState, err := getClients(client)
	if err != nil {
		panic(err)
	}

	// Get channel information.
	channels := make(map[int]string)
	for i := range newState {
		find := false
		for k, v := range channels {
			if newState[i].CID == k {
				newState[i].ChannelName = v
				find = true
				break
			}
		}
		if !find {
			channelInfo, err := getChannelInfo(client, newState[i].CID)
			if err != nil {
				panic(err)
			}
			channelName := ts3.Unescape(channelInfo["channel_name"])
			channels[newState[i].CID] = channelName
			newState[i].ChannelName = channelName
		}
	}

	// If output file is not exist, store state and exit.
	if _, err := os.Stat(output); err != nil {
		if err := storeClients(newState, output); err != nil {
			panic(err)
		}
		os.Exit(0)
	}

	// Getting old state from output file.
	oldState, err := loadClients(output)
	if err != nil {
		panic(err)
	}

	// Prepare to notify new clients.
	var newClients []Client
	for i := range newState {
		if isExist, old := findClient(&newState[i], oldState); isExist && !old.IsNotified {
			newClients = append(newClients, newState[i])
			newState[i].IsNotified = true
		} else if isExist && old.CID != newState[i].CID {
			newClients = append(newClients, newState[i])
			newState[i].IsNotified = true
		} else if isExist && old.IsNotified {
			newState[i].IsNotified = true
		}
	}

	// Notify logged in clients.
	if len(newClients) != 0 {
		if debug {
			// Debug
			channelMap := makeChannelMap(newClients)
			text := buildText(channelMap, true)
			log.Printf(text)
		} else {
			if err := notifyNewClients(webhookURL, newClients); err != nil {
				panic(err)
			}
		}
	}

	// Prepare to notify leaved clients.
	var leavedClients []Client
	for i := range oldState {
		if oldState[i].IsNotified && !matchClient(&oldState[i], newState) {
			leavedClients = append(leavedClients, oldState[i])
		}
	}

	if len(leavedClients) != 0 {
		if debug {
			// Debug
			channelMap := makeChannelMap(leavedClients)
			text := buildText(channelMap, false)
			log.Printf(text)
		} else {
			// Notify leaved clients.
			if err := notifyLeavedClients(webhookURL, leavedClients); err != nil {
				panic(err)
			}
		}
	}

	// Store new state.
	err = storeClients(newState, output)
	if err != nil {
		panic(err)
	}
}

func notifyNewClients(url string, clients []Client) error {
	channelMap := makeChannelMap(clients)
	text := buildText(channelMap, true)
	if text == "" {
		return nil
	}
	err := postToSlack(url, text)
	if err != nil {
		return err
	}
	return nil
}

func notifyLeavedClients(url string, clients []Client) error {
	channelMap := makeChannelMap(clients)
	text := buildText(channelMap, false)
	if text == "" {
		return nil
	}
	err := postToSlack(url, text)
	if err != nil {
		return err
	}
	return nil
}

func makeChannelMap(clients []Client) map[string][]string {

	// Make map from clients based on channels.

	channelMap := make(map[string][]string)
	for i := range clients {
		channelMap[clients[i].ChannelName] = append(channelMap[clients[i].ChannelName], clients[i].Name)
	}
	return channelMap
}

func buildText(info map[string][]string, login bool) string {

	// Build text from channel map.

	var w bytes.Buffer
	for k, v := range info {
		for i := range v {
			w.WriteString(v[i])
			if i != len(v)-1 {
				if i == len(v)-2 {
					w.WriteString(" and ")
				} else {
					w.WriteString(", ")
				}
			}
		}

		if login {
			w.WriteString(" connected to ")
		} else {
			w.WriteString(" disconnected from ")
		}
		w.WriteString(k)
		w.WriteString("\n")
	}
	return w.String()
}

func postToSlack(url, text string) error {

	// Posting slack incoming webhooks.

	msg := &inco.Message{
		Text: text,
	}

	return inco.Incoming(url, msg)
}

func matchClient(target *Client, clientList []Client) (isExist bool) {

	// Search for client is exist in the list.

	for i := range clientList {
		if target.CliID == clientList[i].CliID {
			isExist = true
			break
		}
	}
	return
}

func findClient(target *Client, clientList []Client) (isExist bool, found *Client) {

	// Search for client is exist and the instance.

	for i := range clientList {
		if target.CliID == clientList[i].CliID {
			isExist = true
			found = &clientList[i]
			break
		}
	}
	return
}

func getClients(client *ts3.Client) (res []Client, err error) {

	// Get client information from the virtual server.

	r, err := client.Exec(ts3.ClientList())
	if err != nil {
		return nil, err
	}

	for _, param := range r.Params {
		if clientType, ok := param["client_type"]; ok && clientType == "0" {
			cli, err := NewClientFromMap(param)
			if err != nil {
				return nil, err
			}
			res = append(res, *cli)
		}
	}
	return
}

func getChannelInfo(client *ts3.Client, cid int) (map[string]string, error) {

	// Getting information of the channel.

	cmd := ts3.Command{
		Command: "channelinfo",
		Params: map[string][]string{
			"cid": []string{strconv.Itoa(cid)},
		},
	}

	r, err := client.Exec(cmd)
	if err != nil {
		return nil, err
	}

	return r.Params[0], nil
}

func loadClients(fileName string) ([]Client, error) {

	// Loading clients from the file.

	b, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	var clients []Client
	err = json.Unmarshal(b, &clients)
	if err != nil {
		return nil, err
	}
	return clients, nil
}

func storeClients(clients []Client, fileName string) error {

	// Saving clients into the file.

	b, err := json.Marshal(clients)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(fileName, b, 0644)
	if err != nil {
		return err
	}
	return nil
}
