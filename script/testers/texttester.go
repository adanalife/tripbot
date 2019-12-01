package main

import (
	"github.com/sfreiberg/gotwilio"
)

func main() {
	accountSid := "AC9dde3922a67cfa97d7819437e0099a8f"
	authToken := "ab1d9695e161327d053fa4455d928a2e"
	twilio := gotwilio.NewTwilioClient(accountSid, authToken)

	from := "+17813861142"
	to := "+19782061331"
	message := "Welcome to gotwilio!"
	twilio.SendSMS(from, to, message, "", "")
}
