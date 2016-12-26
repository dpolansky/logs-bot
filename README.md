# logs-bot

logs-bot is a simple Twitch bot that fetches TF2 match logs from logs.tf and posts them in a Twitch chat room.

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