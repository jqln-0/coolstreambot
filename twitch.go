package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
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

var ceilingBulb, bedBulb *golifx.Bulb

func verifyWebhook(r *http.Request, requestBody []byte, hmacKey []byte) bool {
	signatureList, ok := r.Header["X-Hub-Signature"]
	if !ok {
		log.Println("missing signature")
		return false
	}
	if len(signatureList) != 1 {
		log.Println("multiple signatures")
		return false
	}
	splitSignature := strings.SplitN(signatureList[0], "=", 2)
	if len(splitSignature) != 2 {
		log.Println("malformed signature")
		return false
	}

	method, hexSignature := splitSignature[0], splitSignature[1]
	signature, err := hex.DecodeString(hexSignature)
	if err != nil {
		log.Println("malformed signature: could not decode hex")
		return false
	}

	var hasher func() hash.Hash
	switch method {
	case "sha1":
		hasher = sha1.New
	case "sha256":
		hasher = sha256.New
	case "sha384":
		hasher = sha512.New384
	case "sha512":
		hasher = sha512.New
	default:
		log.Println("unknown signature algorithm", method)
		return false
	}

	hmac := hmac.New(hasher, hmacKey)
	hmac.Write(requestBody)
	expectedMAC := hmac.Sum(nil)
	return bytes.Equal(expectedMAC, signature)
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	msgType, ok := r.Header["Twitch-Eventsub-Message-Type"]
	if !ok {
		log.Println("missing message type", r.Header)
		return
	}
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("failed to read body", err)
		return
	}

	hmacKey := []byte(os.Getenv("sub_secret"))
	if !verifyWebhook(r, requestBody, hmacKey) {
		log.Println("failed to verify signature")
		return
	}

	var payload PayloadJson
	err = json.NewDecoder(bytes.NewReader(requestBody)).Decode(&payload)
	if err != nil {
		log.Println("bad body", err)
		return
	}
	if msgType[0] == "webhook_callback_verification" {
		log.Printf("got verification callback, challenge %s", payload.Challenge)
		w.Write([]byte(payload.Challenge))
		return
	}
	if msgType[0] == "notification" {
		reward := payload.Event.Reward.Title
		params := payload.Event.UserInput
		if reward == "lights" {
			number, err := strconv.ParseUint(params, 10, 64)
			if err != nil {
				hasher := crc32.NewIEEE()
				hasher.Write([]byte(params))
				number = uint64(hasher.Sum32())
			}
			whichBulb := (number / 65535) % 2
			bulb := []*golifx.Bulb{bedBulb, ceilingBulb}[whichBulb]
			hue := number % 65535
			col := &golifx.HSBK{
				Hue:        uint16(hue),
				Saturation: 65535,
				Brightness: 65535,
				Kelvin:     3200,
			}
			log.Printf("setting bulb %d to %d", whichBulb, hue)
			bulb.SetColorState(col, 1)
		} else if reward == "end the stream" {
			log.Print("killing stream")
			cmd := exec.Command("killall", "obs")
			cmd.Run()
		} else if reward == "silence me" {
			log.Print("ur muted")
			cmd := exec.Command("./silencethot.sh")
			go cmd.Run()
		} else if reward == "SimpBucks Premium" {
			chance := rand.Intn(10)
			sound := "woof.mp3"
			if chance == 5 {
				sound = "bark.mp3"
			}
			cmd := exec.Command("play", sound)
			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env, "AUDIODEV=hw:1,0")
			go cmd.Run()
		} else if reward == "scrollo" {
			f, err := os.Create("scrollo.txt")
			defer f.Close()
			if err != nil {
				log.Printf("failed to create file: %s", err)
				return
			}
			f.WriteString(fmt.Sprintf(" %.256s ✨✨✨ ", params))
		}
		return
	}
	log.Printf("got something else! %s", msgType)
}

func main() {
	log.Println("finding bulbs")
	bulbs, err := golifx.LookupBulbs()
	if err != nil {
		log.Fatalf("failed to find bulbs! %s", err)
		return
	}
	for _, bulb := range bulbs {
		mac := bulb.MacAddress()
		if mac == "d0:73:d5:64:76:ac" {
			log.Println("found ceiling bulb")
			ceilingBulb = bulb
		} else if mac == "d0:73:d5:66:d5:ec" {
			log.Println("found bed bulb")
			bedBulb = bulb
		}
	}
	if ceilingBulb == nil || bedBulb == nil {
		log.Fatalf("missing bulb(s). bulbs: %s %s", ceilingBulb, bedBulb)
	}

	log.Println("starting server")
	http.HandleFunc("/webhook", handleWebhook)
	log.Fatal(http.ListenAndServeTLS(":6969", "cert/config/live/cardassia.jacqueline.id.au/fullchain.pem", "cert/config/live/cardassia.jacqueline.id.au/privkey.pem", nil))
}
