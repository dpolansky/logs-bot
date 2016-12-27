package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	staleLogThresholdInSeconds    = 30
	elapsedTimeThresholdInSeconds = 60
	spoilerSleepTimeInSeconds     = 15
	logRefreshTimeInSeconds       = 10
	twitchIRCRetryTimeInSeconds   = 30

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

	mutex             *sync.Mutex
	steamIDToLastTime map[string]time.Time

	userName string
	oauthKey string
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
		mutex:             &sync.Mutex{},
		steamIDToLastTime: map[string]time.Time{},
	}

	for {
		err := b.Serve()
		log.Printf("Error serving: %v, retrying in %v seconds\n", err, twitchIRCRetryTimeInSeconds)
		time.Sleep(twitchIRCRetryTimeInSeconds * time.Second)
	}
}

func (b *botConfig) Serve() error {
	log.Printf("Connecting to Twitch IRC server\n")
	if err := b.connect(); err != nil {
		return fmt.Errorf("Failed to connect to Twitch IRC server\n")
	}

	log.Printf("Connected!\n")

	die := make(chan struct{})
	go func(die chan struct{}) {
		for {
			select {
			case <-die:
				return
			default:
				for steamid, channel := range b.steamIDToTwitchChannel {
					go b.checkLogsForPlayer(steamid, channel)
				}
				time.Sleep(logRefreshTimeInSeconds * time.Second)
			}
		}
	}(die)

	err := b.readMessages()
	die <- struct{}{}
	return err
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

func (b *botConfig) checkLogsForPlayer(steamid, channel string) error {
	res, err := getNewestLogForPlayer(steamid)
	if err != nil {
		return err
	}

	id := strconv.Itoa(res.ID)
	tm := time.Unix(res.Date, 0)
	elapsed := time.Since(tm)

	b.mutex.Lock()
	defer b.mutex.Unlock()
	lastTime, _ := b.steamIDToLastTime[steamid]

	// ignore logs that are stale
	// stale if:
	// the log is older than the stale log threshold
	// the time elapsed since the last log was sent is less than elapsed threshold
	if elapsed.Seconds() > staleLogThresholdInSeconds || time.Now().Sub(lastTime).Seconds() < elapsedTimeThresholdInSeconds {
		return nil
	}

	// sleep to prevent spoilers
	time.Sleep(spoilerSleepTimeInSeconds * time.Second)

	_, err = fmt.Fprintf(b.conn, "PRIVMSG #"+channel+" :http://logs.tf/"+id+"\r\n")
	if err != nil {
		return err
	}

	log.Printf("Sent log id=%v channel=%v\n", id, channel)
	b.steamIDToLastTime[steamid] = time.Now()
	return nil
}

func getNewestLogForPlayer(steamid string) (*logResponse, error) {
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
		return nil, fmt.Errorf("Failed to get log for steamid=%v, response:\n%v\n", steamid, string(body))
	}

	return &(q.Logs[0]), nil
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
