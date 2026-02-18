package main

var Config = struct {
	OwnerNumber   string
	BotName       string
	OTPChannelIDs []string
	OTPApiURLs    []string
	Interval      int
}{
	OwnerNumber: "923027665767",
	BotName:     "Kami OTP Monitor",
	OTPChannelIDs: []string{
		"120363423562861659@newsletter",
		"120363421646654726@newsletter",
	},
	OTPApiURLs: []string{
		"https://kami-api.up.railway.app/npm-neon/sms",
		"https://kami-api.up.railway.app/mait/sms",
	},
	Interval: 6,
}
