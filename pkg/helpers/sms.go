package helpers

import (
	"log"
	"os"
	"sync"

	"github.com/sfreiberg/gotwilio"
)

var (
	twilioOnce    sync.Once
	twilioClient  *gotwilio.Twilio
	twilioFromNum string
	twilioToNum   string
)

// twilio constructs (on first call) and returns the Twilio client.
// It log.Fatals if the required env vars are missing — but only when
// SMS is actually being sent, so importing this package no longer
// fails at startup for binaries that never send SMS.
func twilio() *gotwilio.Twilio {
	twilioOnce.Do(func() {
		requiredVars := []string{
			"TWILIO_ACCT_SID",
			"TWILIO_AUTH_TOKEN",
			"TWILIO_FROM_NUM",
			"TWILIO_TO_NUM",
		}
		for _, v := range requiredVars {
			_, ok := os.LookupEnv(v)
			if !ok {
				log.Fatalf("You must set %s", v)
			}
		}
		twilioAccountSid := os.Getenv("TWILIO_ACCT_SID")
		twilioAuthToken := os.Getenv("TWILIO_AUTH_TOKEN")
		twilioFromNum = os.Getenv("TWILIO_FROM_NUM")
		twilioToNum = os.Getenv("TWILIO_TO_NUM")

		twilioClient = gotwilio.NewTwilioClient(twilioAccountSid, twilioAuthToken)
	})
	return twilioClient
}

// sends an SMS (to myself)
func SendSMS(message string) {
	twilio().SendSMS(twilioFromNum, twilioToNum, message, "", "")
}
