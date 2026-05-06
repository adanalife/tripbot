package helpers

import (
	"log"
	"os"
	"sync"

	"github.com/sfreiberg/gotwilio"
)

var (
	twilioFromNum, twilioToNum string
	twilioClient               *gotwilio.Twilio
	twilioOnce                 sync.Once
)

func initTwilio() {
	requiredVars := []string{
		"TWILIO_ACCT_SID",
		"TWILIO_AUTH_TOKEN",
		"TWILIO_FROM_NUM",
		"TWILIO_TO_NUM",
	}
	for _, v := range requiredVars {
		if _, ok := os.LookupEnv(v); !ok {
			log.Fatalf("You must set %s", v)
		}
	}
	twilioFromNum = os.Getenv("TWILIO_FROM_NUM")
	twilioToNum = os.Getenv("TWILIO_TO_NUM")
	twilioClient = gotwilio.NewTwilioClient(
		os.Getenv("TWILIO_ACCT_SID"),
		os.Getenv("TWILIO_AUTH_TOKEN"),
	)
}

// sends an SMS (to myself)
func SendSMS(message string) {
	twilioOnce.Do(initTwilio)
	twilioClient.SendSMS(twilioFromNum, twilioToNum, message, "", "")
}
