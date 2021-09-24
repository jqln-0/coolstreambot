package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/2tvenom/golifx"
)

type RewardJson struct {
	Title string `json:"title"`
}

type EventJson struct {
	UserInput string     `json:"user_input"`
	Reward    RewardJson `json:"reward"`
}

type PayloadJson struct {
	Challenge string    `json:"challenge"`
	Event     EventJson `json:"event"`
}

type reward int8

const (
	lights reward = iota
	endStream
	silenceMe
	premium
	scrollo
	comrade
	unknown
)
const (
	signatureHeader = "Twitch-Eventsub-Message-Signature"
	timestampHeader = "Twitch-Eventsub-Message-Timestamp"
	msgIdHeader     = "Twitch-Eventsub-Message-Id"
	msgTypeHeader   = "Twitch-Eventsub-Message-Type"
)

func rewardFromString(s string) reward {
	switch s {
	case "lights":
		return lights
	case "end the stream":
		return endStream
	case "silence me":
		return silenceMe
	case "SimpBucks Premium":
		return premium
	case "scrollo":
		return scrollo
	case "comrade l'egg"
		return comrade
	default:
		return unknown
	}
}

type hmacKey struct {
	secret      []byte
	permissions []reward
}

var knownBulbMacs = []string{
	"d0:73:d5:64:76:ac", // ceiling bulb
	//"d0:73:d5:66:d5:ec" // bed bulb (on loan to daniel)
}
var macToBulb = make(map[string]*golifx.Bulb)

func findAllBulbs() {
	for i := 0; i < 3; i++ {
		foundBulbs, err := golifx.LookupBulbs()
		if err != nil {
			log.Fatalf("error finding bulbs! %s", err)
			return
		}
		for _, foundBulb := range foundBulbs {
			foundMac := foundBulb.MacAddress()
			for _, mac := range knownBulbMacs {
				if mac == foundMac {
					macToBulb[mac] = foundBulb
				}
			}
		}

		if len(macToBulb) == len(knownBulbMacs) {
			return
		}
	}

	// FIXME: ideally we might print the names of the missing bulbs here. effrot.
	log.Fatalf("missing bulb(s)! found %d, wanted %d", len(macToBulb), len(knownBulbMacs))
}

func getCoolHeader(name string, h http.Header) (string, error) {
	val, ok := h[name]
	if !ok {
		return "", fmt.Errorf("missing header %s", name)
	}
	if len(val) != 1 {
		return "", errors.New("too many headers")
	}
	return val[0], nil
}

func verifyWebhook(h http.Header, requestBody []byte, hmacKeys []hmacKey) []reward {
	signatures, err := getCoolHeader(signatureHeader, h)
	if err != nil {
		log.Println(err)
		return nil
	}
	splitSignature := strings.SplitN(signatures, "=", 2)
	if len(splitSignature) != 2 {
		log.Println("malformed signature")
		return nil
	}

	hexSignature := splitSignature[1]
	signature, err := hex.DecodeString(hexSignature)
	if err != nil {
		log.Println("malformed signature: could not decode hex")
		return nil
	}

	timestamp, err := getCoolHeader(timestampHeader, h)
	if err != nil {
		log.Println(err)
		return nil
	}

	msgId, err := getCoolHeader(msgIdHeader, h)
	if err != nil {
		log.Println(err)
		return nil
	}
	for _, hmacKey := range hmacKeys {
		calculatedHMAC := hmac.New(sha256.New, hmacKey.secret)
		calculatedHMAC.Write([]byte(msgId))
		calculatedHMAC.Write([]byte(timestamp))
		calculatedHMAC.Write(requestBody)
		expectedMAC := calculatedHMAC.Sum(nil)
		if hmac.Equal(expectedMAC, signature) {
			return hmacKey.permissions
		}
	}
	return nil
}

func lightsReward(params string) {
	number, err := strconv.ParseUint(params, 10, 64)
	if err != nil {
		hasher := crc32.NewIEEE()
		hasher.Write([]byte(params))
		number = uint64(hasher.Sum32())
	}
	whichBulb := (number / 65535) % uint64(len(knownBulbMacs))
	bulb := macToBulb[knownBulbMacs[whichBulb]]
	hue := number % 65535
	col := &golifx.HSBK{
		Hue:        uint16(hue),
		Saturation: 65535,
		Brightness: 65535,
		Kelvin:     3200,
	}
	log.Printf("setting bulb %d to %d", whichBulb, hue)
	bulb.SetColorState(col, 1)
}

func endStreamReward() {
	log.Print("killing stream")
	cmd := exec.Command("killall", "obs")
	cmd.Run()
}

func silenceMeReward() {
	log.Print("ur muted")
	cmd := exec.Command("./silencethot.sh")
	go cmd.Run()
}

func premiumReward() {
	chance := rand.Intn(10)
	sound := "woof.mp3"
	if chance == 5 {
		sound = "bark.mp3"
	}
	cmd := exec.Command("play", sound)
	cmd.Env = os.Environ()
	// FIXME: Make this configurable via a flag.
	// For Pokemon Emerald, the default audio device is correct.
	//cmd.Env = append(cmd.Env, "AUDIODEV=hw:1,0")
	go cmd.Run()
}

func scrolloReward(params string) {
	f, err := os.Create("scrollo.txt")
	if err != nil {
		log.Printf("failed to create file: %s", err)
		return
	}
	defer f.Close()
	f.WriteString(fmt.Sprintf(" %.256s ✨✨✨ ", strings.ToValidUTF8(params, "⚠️ nice try, puppy⚠️ ")))
}

func premiumReward() {
	cmd := exec.Command("play", "song.mp3")
	go cmd.Run()
}

func unknownReward(reward string) {
	log.Printf("request for unknown reward %s", reward)
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("failed to read body", err)
		return
	}

	domKey := hmacKey{[]byte(os.Getenv("dom_secret")), []reward{lights, scrollo, unknown}}
	subKey := hmacKey{[]byte(os.Getenv("sub_secret")), []reward{lights, endStream, silenceMe, premium, scrollo, comrade, unknown}}
	hmacKeys := []hmacKey{domKey, subKey}
	permissions := verifyWebhook(r.Header, requestBody, hmacKeys)
	if permissions == nil {
		log.Println("failed to verify signature")
		w.Write([]byte("it's something different tonight, puppy!"))
		return
	}
	msgType, err := getCoolHeader(msgTypeHeader, r.Header)
	if err != nil {
		log.Println(err)
		return
	}

	var payload PayloadJson
	err = json.NewDecoder(bytes.NewReader(requestBody)).Decode(&payload)
	if err != nil {
		log.Println("bad body", err)
		return
	}
	if msgType == "webhook_callback_verification" {
		log.Printf("got verification callback, challenge %s", payload.Challenge)
		w.Write([]byte(payload.Challenge))
		return
	}
	if msgType == "notification" {
		reward := rewardFromString(payload.Event.Reward.Title)
		params := payload.Event.UserInput
		rewardInPermissions := false
		for _, allowed := range permissions {
			if reward == allowed {
				rewardInPermissions = true
				break
			}
		}
		if !rewardInPermissions {
			log.Print("reward requested not in authorised rewards")
			return
		}
		log.Printf("fulfilling reward %s", payload.Event.Reward.Title)
		switch reward {
		case lights:
			lightsReward(params)
		case endStream:
			endStreamReward()
		case silenceMe:
			silenceMeReward()
		case premium:
			premiumReward()
		case scrollo:
			scrolloReward(params)
		case unknown:
			unknownReward(payload.Event.Reward.Title)
		}
		return
	}
	log.Printf("got something else! %s", msgType)
}

func main() {
	log.Println("finding bulbs")
	findAllBulbs()
	log.Println("starting server")
	http.HandleFunc("/webhook", handleWebhook)
	log.Fatal(http.ListenAndServeTLS(":6969", "cert/config/live/cardassia.jacqueline.id.au/fullchain.pem", "cert/config/live/cardassia.jacqueline.id.au/privkey.pem", nil))
}
