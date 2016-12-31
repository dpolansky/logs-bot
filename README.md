# logs-bot

logs-bot is a simple Twitch bot that fetches TF2 match logs from logs.tf and posts them in a Twitch chat room.
![Chat Screenshot](https://raw.githubusercontent.com/dpolansky/logs-bot/master/chat-screenshot.png)

## Installation

```
go get -u github.com/dpolansky/logs-bot
```

## Usage
Set environment variables for the Twitch bot's username and oauthkey (which can be found [here](https://twitchapps.com/tmi/))

```
export LOGS_BOT_USERNAME="LogsTFBot"
export LOGS_BOT_OAUTH_KEY=key
```

Create a file `channels.json` with a mapping from each steamID to Twitch channel name:
```javascript
{
  "76561198107240606": "lansky",
  "76561197991735941": "clockwork"
}
```

`go get` will create an executable in your `$GOPATH/bin` (make sure `$GOPATH/bin` is in your $PATH), or build the executable with:
```
go build
```

Then run the executable:
```
logs-bot
```