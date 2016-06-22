# ts3-slack-bot

ts3-slack-bot detects logging to Team Speak server and posts it to slack, incoming webhook.

# Usage

```bash
Usage of ./ts3-slack-bot:
  -d  Debug
  -id string
    Server ID
  -o string
    Output file (default "clients.json")
  -p string
    TS3 server query password
  -u string
    TS3 server query username
  -url string
    WebHookURL
```

0. Get the source code and build it.

```bash
go get github.com/Rompei/ts3-slack-bot
cd $GOPATH/src/github.com/Rompei/ts3-slack-bot
go build
```

1. Set the software with cron. In this example, it runs every five minutes.

```bash
*/5 * * * * ts3-slack-bot [OPTIONS]
```

2. First time, it gets client information from team speak server with server query and stores it.

3. Next time, it gets client information and compares with old one, then posts client statuses to Slack. In this example, it will post 5 minutes later of first step.

# Features
- If people enter the server, it will notify it within from 5 to 10 minutes
- If people leave the server, it will notify it.
- If people change channels, it will notify it.

# Used libraries

[toqueteos/ts3](https://github.com/toqueteos/ts3)

# License

[BSD-3](https://opensource.org/licenses/BSD-3-Clause)
