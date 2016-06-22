package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/toqueteos/ts3"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Client is object of client.
type Client struct {
	CliID       int    `json:"cliId"`
	CID         int    `json:"cId"`
	Name        string `json:"name"`
	ChannelName string `json:"channelName"`
	IsNotified  bool   `json:"isNotified"`
}

// WebHookBody is body of slack webhook.
type WebHookBody struct {
	Text      string `json:"text"`
	Channel   string `json:"channel"`
	Username  string `json:"username"`
	IconEmoji string `json:"icon_emoji"`
}

// NewClient is constructor of Client.
func NewClient(cliID, cID int, name string) *Client {
	return &Client{
		CliID: cliID,
		CID:   cID,
		Name:  name,
	}
}

func main() {

	var (
		username   string
		password   string
		serverID   string
		webhookURL string
		output     string
		debug      bool
	)

	flag.StringVar(&username, "u", "", "TS3 server query username")
	flag.StringVar(&password, "p", "", "TS3 server query password")
	flag.StringVar(&serverID, "id", "", "Server ID")
	flag.StringVar(&webhookURL, "url", "", "WebHookURL")
	flag.StringVar(&output, "o", "clients.json", "Output file")
	flag.BoolVar(&debug, "d", false, "Debug")
	flag.Parse()

	if username == "" || password == "" || serverID == "" || webhookURL == "" {
		panic(fmt.Errorf("Not enough options"))
	}

	conn, err := ts3.Dial(":10011", true)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// Login to the server.
	err = initConn(conn, username, password)
	if err != nil {
		panic(err)
	}

	// Select a server.
	err = connectToServer(conn, serverID)
	if err != nil {
		panic(err)
	}

	// Get client list.
	newState, err := getClients(conn)
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
			channelInfo, err := getChannelInfo(conn, newState[i].CID)
			if err != nil {
				panic(err)
			}
			channelName := ts3.Unquote(channelInfo["channel_name"])
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
			fmt.Println(text)
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
			fmt.Println(text)
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
				w.WriteString(", ")
			}
		}
		if login {
			w.WriteString(" connected to ")
		} else {
			w.WriteString(" left ")
		}
		w.WriteString(k)
		w.WriteString("\n")
	}
	return w.String()
}

func postToSlack(url, text string) error {

	// Posting slack incoming webhooks.

	body := WebHookBody{
		Text: text,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(b)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
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

func handleError(err *ts3.ErrorMsg) error {
	if err.Id != 0 {
		return fmt.Errorf(err.Msg)
	}
	return nil
}

func initConn(conn *ts3.Conn, username, password string) error {

	// Login to team speak server query.

	_, err := conn.Cmd(fmt.Sprintf("login %s %s", username, password))
	return handleError(&err)
}

func connectToServer(conn *ts3.Conn, serverID string) error {

	// Connect to the virtual server.

	_, err := conn.Cmd(fmt.Sprintf("use %s", serverID))
	return handleError(&err)
}

func getClients(conn *ts3.Conn) (res []Client, err error) {

	// Get client information from the virtual server.

	r, errMsg := conn.Cmd("clientlist")
	if err := handleError(&errMsg); err != nil {
		return nil, err
	}
	clients := strings.Split(r, "|")
	for i := range clients {
		cliParams := mappingParams(clients[i])
		if cliParams["client_type"] == "0" {
			clid, err := strconv.Atoi(cliParams["clid"])
			if err != nil {
				return nil, err
			}
			cid, err := strconv.Atoi(cliParams["cid"])
			if err != nil {
				return nil, err
			}
			res = append(res, *NewClient(clid, cid, ts3.Unquote(cliParams["client_nickname"])))
		}
	}
	return
}

func getChannelInfo(conn *ts3.Conn, cid int) (map[string]string, error) {

	// Getting information of the channel.

	r, errMsg := conn.Cmd(fmt.Sprintf("channelinfo cid=%d", cid))
	if err := handleError(&errMsg); err != nil {
		return nil, err
	}
	return mappingParams(r), nil
}

func mappingParams(obj string) (params map[string]string) {

	// Mapping response of team speak server query.

	params = make(map[string]string)
	info := strings.Fields(obj)
	for i := range info {
		pair := strings.Split(info[i], "=")
		if len(pair) != 2 {
			continue
		}
		params[pair[0]] = pair[1]
	}
	return
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
