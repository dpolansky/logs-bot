package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	staleLogThresholdInMinutes  = 1
	spoilerSleepTimeInSeconds   = 15
	logRefreshTimeInSeconds     = 10
	twitchIRCRetryTimeInSeconds = 30

	twitchIRCHostPort = "irc.chat.twitch.tv:6667"

	channelsFileName = "channels.json"

	userNameEnvName = "LOGS_BOT_USERNAME"
	oauthKeyEnvName = "LOGS_BOT_OAUTH_KEY"
)

type logResponse struct {
	Date  int64  `json:"date"`
	ID    int    `json:"id"`
	Title string `json:"title"`
}

type botConfig struct {
	conn                   net.Conn
	steamIDToTwitchChannel map[string]string
	userName               string
	oauthKey               string
}

func main() {
	userName := os.Getenv(userNameEnvName)
	oauthKey := os.Getenv(oauthKeyEnvName)

	if userName == "" || oauthKey == "" {
		log.Printf("Environment variables %v and %v not set.", userNameEnvName, oauthKeyEnvName)
		os.Exit(1)
	}

	steamIDToTwitchChannel, err := loadChannelsFromFile()
	if err != nil {
		log.Printf("Failed to load channels from %v: %v\n", channelsFileName, err)
		os.Exit(1)
	}

	b := &botConfig{
		userName:               userName,
		oauthKey:               oauthKey,
		steamIDToTwitchChannel: steamIDToTwitchChannel,
	}

	b.Serve()
}

func loadChannelsFromFile() (map[string]string, error) {
	var channels map[string]string
	b, err := ioutil.ReadFile(channelsFileName)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(b, &channels)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

func (b *botConfig) Serve() {
	for {
		shutdownChannels := []chan struct{}{}

		if err := b.connect(); err != nil {
			log.Printf("Failed to connect to Twitch IRC server, retrying")
			time.Sleep(twitchIRCRetryTimeInSeconds * time.Second)
			continue
		}

		log.Printf("Connected to Twitch IRC server\n")

		for steamid, channel := range b.steamIDToTwitchChannel {
			c := make(chan struct{})
			shutdownChannels = append(shutdownChannels, c)
			go b.handleLogsForPlayer(steamid, channel, c)
		}

		if err := b.readMessages(); err != nil {
			log.Printf("Error in reading message from Twitch: %v, reconnecting\n", err)
		}

		// shut down worker go-routines
		for _, ch := range shutdownChannels {
			ch <- struct{}{}
		}
	}
}

func (b *botConfig) readMessages() error {
	tp := textproto.NewReader(bufio.NewReader(b.conn))

	for {
		time.Sleep(1 * time.Second)

		line, err := tp.ReadLine()
		if err != nil {
			return err
		}

		// respond to pings to keep the bot alive
		if strings.Contains(line, "PING") {
			fmt.Fprintf(b.conn, "PONG :tmi.twitch.tv\r\n")
		}
	}
}

func (b *botConfig) handleLogsForPlayer(steamid, channel string, shutdown chan struct{}) {
	fmt.Fprintf(b.conn, "JOIN #%s\r\n", channel)
	log.Printf("Connected to channel: %v\n", channel)

	lastLog := ""
	done := false

	for !done {
		select {
		case <-shutdown:
			done = true
			continue
		default:
			break
		}

		time.Sleep(logRefreshTimeInSeconds * time.Second)

		res, err := getLastLogForPlayer(steamid)
		if err != nil {
			log.Printf("Failed to get log for channel=%s: %v\n", channel, err)
			continue
		}

		tm := time.Unix(res.Date, 0)
		elapsed := time.Since(tm)
		id := strconv.Itoa(res.ID)

		// ignore logs that are stale
		if id == lastLog || elapsed.Minutes() > staleLogThresholdInMinutes {
			continue
		}

		// sleep to prevent spoilers
		time.Sleep(spoilerSleepTimeInSeconds * time.Second)

		fmt.Fprintf(b.conn, "PRIVMSG #"+channel+" :http://logs.tf/"+id+" (%v min ago)\r\n", math.Ceil(elapsed.Minutes()))
		log.Printf("sent log id=%v channel=%v minutesElapsed=%v\n", id, channel, math.Ceil(elapsed.Minutes()))

		lastLog = id
	}

	log.Printf("Shutting down worker for channel: %v\n", channel)
}

func (b *botConfig) connect() error {
	conn, err := net.Dial("tcp", twitchIRCHostPort)
	if err != nil {
		return err
	}

	fmt.Fprintf(conn, "PASS %s\r\n", string(b.oauthKey))
	fmt.Fprintf(conn, "NICK %s\r\n", b.userName)

	b.conn = conn
	return nil
}

func getLastLogForPlayer(steamid string) (*logResponse, error) {
	res, err := http.Get("http://logs.tf/json_search?player=" + steamid + "&limit=1")

	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		return nil, err
	}

	type queryResponse struct {
		Logs    []logResponse `json:"logs"`
		Results int           `json:"results"`
		Success bool          `json:"success"`
	}

	var q queryResponse
	err = json.Unmarshal(body, &q)

	if err != nil {
		return nil, err
	}

	if q.Success == false || q.Results == 0 {
		return nil, fmt.Errorf("query failed, results=%v\n", q.Results)
	}

	return &(q.Logs[0]), nil
}
