package helpers

import (
	"os"

	"github.com/sfreiberg/gotwilio"
)

var twilioFromNum, twilioToNum string
var twilioClient *gotwilio.Twilio

// set up Twilio (for text messages)
func init() {
	twilioAccountSid := os.Getenv("TWILIO_ACCT_SID")
	if twilioAccountSid == "" {
		panic("You must set TWILIO_ACCT_SID")
	}
	twilioAuthToken := os.Getenv("TWILIO_AUTH_TOKEN")
	if twilioAuthToken == "" {
		panic("You must set TWILIO_AUTH_TOKEN")
	}
	twilioFromNum = os.Getenv("TWILIO_FROM_NUM")
	if twilioFromNum == "" {
		panic("You must set TWILIO_FROM_NUM")
	}
	twilioToNum = os.Getenv("TWILIO_TO_NUM")
	if twilioToNum == "" {
		panic("You must set TWILIO_TO_NUM")
	}
	twilioClient = gotwilio.NewTwilioClient(twilioAccountSid, twilioAuthToken)
}

// sends an SMS (to myself)
func SendSMS(message string) {
	twilioClient.SendSMS(twilioFromNum, twilioToNum, message, "", "")
}
