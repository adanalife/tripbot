package helpers

import (
	"log"
	"os"

	"github.com/sfreiberg/gotwilio"
)

var twilioFromNum, twilioToNum string
var twilioClient *gotwilio.Twilio

// set up Twilio (for text messages)
func init() {
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
}

// sends an SMS (to myself)
func SendSMS(message string) {
	twilioClient.SendSMS(twilioFromNum, twilioToNum, message, "", "")
}
