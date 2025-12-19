package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

var client *whatsmeow.Client
var lastProcessedIDs = make(map[string]bool)

// --- مددگار فنکشنز ---
func extractOTP(msg string) string {
	re := regexp.MustCompile(`\b\d{3,4}[-\s]?\d{3,4}\b|\b\d{4,8}\b`)
	return re.FindString(msg)
}

func maskNumber(num string) string {
	if len(num) < 7 { return num }
	return num[:5] + "XXXX" + num[len(num)-2:]
}

// --- اے پی آئی مانیٹرنگ (بغیر بٹنز کے - جیسا آپ نے کہا) ---
func checkOTPs(cli *whatsmeow.Client) {
	for _, url := range Config.OTPApiURLs {
		resp, err := http.Get(url)
		if err != nil { continue }
		defer resp.Body.Close()

		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil { continue }

		aaData, ok := data["aaData"].([]interface{})
		if !ok { continue }

		for _, row := range aaData {
			r, ok := row.([]interface{})
			if !ok || len(r) < 5 { continue }

			msgID := fmt.Sprintf("%v_%v", r[2], r[0])
			if !lastProcessedIDs[msgID] {
				rawTime, _ := r[0].(string)
				countryInfo, _ := r[1].(string)
				phone, _ := r[2].(string)
				service, _ := r[3].(string)
				fullMsg, _ := r[4].(string)

				cFlag, countryWithFlag := GetCountryWithFlag(countryInfo)
				otpCode := extractOTP(fullMsg)

				messageBody := fmt.Sprintf(`
✨ *%s | %s New Message*⚡
> ⏰ *Time:* _%s_
> 🌍 *Country:* _%s_
> 📞 *Number:* _%s_
> ⚙️ *Service:* _%s_
> 🔑 *OTP:* *%s*

📩 *Full Message:*
"%s"

_Developed by Nothing Is Impossible_
`, cFlag, strings.ToUpper(service), rawTime, countryWithFlag, maskNumber(phone), service, otpCode, fullMsg)

				for _, jidStr := range Config.OTPChannelIDs {
					jid, _ := types.ParseJID(jidStr)
					cli.SendMessage(context.Background(), jid, &waProto.Message{
						Conversation: proto.String(strings.TrimSpace(messageBody)),
					})
				}
				lastProcessedIDs[msgID] = true
			}
		}
	}
}

// --- بٹن ٹیسٹ کرنے کا مین فنکشن ---
func sendTestButtons(cli *whatsmeow.Client, chat types.JID) {
	fmt.Printf("🛠 [Test] Sending multiple button styles to %s...\n", chat)

	// 1. ریپلائی بٹنز (Buttons Message - Legacy)
	replyButtons := &waProto.ButtonsMessage{
		ContentText: proto.String("Style 1: Quick Reply Buttons"),
		HeaderType:  waProto.ButtonsMessage_EMPTY.Enum(),
		Buttons: []*waProto.Button{
			{ButtonId: proto.String("btn1"), ButtonText: &waProto.ButtonText{DisplayText: proto.String("Yes")}},
			{ButtonId: proto.String("btn2"), ButtonText: &waProto.ButtonText{DisplayText: proto.String("No")}},
		},
	}

	// 2. لسٹ بٹن (3-Line Button / List Message)
	listMessage := &waProto.ListMessage{
		Title:       proto.String("Style 2: List Options"),
		Description: proto.String("Click the button below to see the list"),
		ButtonText:  proto.String("Open Menu"),
		ListType:    waProto.ListMessage_SINGLE_SELECT.Enum(),
		Sections: []*waProto.ListSection{
			{
				Title: proto.String("Our Services"),
				Rows: []*waProto.ListRow{
					{Title: proto.String("Check ID"), RowId: proto.String("id_row"), Description: proto.String("Get current Chat ID")},
					{Title: proto.String("Check Balance"), RowId: proto.String("bal_row")},
				},
			},
		},
	}

	// 3. ماڈرن بٹنز (Interactive Native Flow - Call to Action)
	interactiveMessage := &waProto.InteractiveMessage{
		Header: &waProto.InteractiveMessage_Header{
			Title: proto.String("Style 3: Native Flow Buttons"),
		},
		Body: &waProto.InteractiveMessage_Body{
			Text: proto.String("Click below to Copy or Visit Link"),
		},
		InteractiveMessageConfig: &waProto.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waProto.InteractiveMessage_NativeFlowMessage{
				Buttons: []*waProto.InteractiveMessage_NativeFlowMessage_Button{
					{
						Name: proto.String("cta_copy"),
						ButtonParamsJson: proto.String(`{"display_text":"Copy OTP Code","id":"copy_123","copy_code":"123-456"}`),
					},
					{
						Name: proto.String("cta_url"),
						ButtonParamsJson: proto.String(`{"display_text":"Join Group","url":"https://chat.whatsapp.com/EbaJKbt5J2T6pgENIeFFht"}`),
					},
				},
			},
		},
	}

	// تمام باری باری بھیجیں اور پرنٹ کریں
	messages := []*waProto.Message{
		{ButtonsMessage: replyButtons},
		{ListMessage: listMessage},
		{ViewOnceMessage: &waProto.ViewOnceMessage{Message: &waProto.Message{InteractiveMessage: interactiveMessage}}},
	}

	for i, msg := range messages {
		resp, err := cli.SendMessage(context.Background(), chat, msg)
		if err != nil {
			fmt.Printf("❌ [Button %d Error]: %v\n", i+1, err)
		} else {
			fmt.Printf("✅ [Button %d Success]: Message ID %s sent\n", i+1, resp.ID)
		}
	}
}

// --- ایونٹ ہینڈلر ---
func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		msgText := v.Message.GetConversation()
		if msgText == "" { msgText = v.Message.GetExtendedTextMessage().GetText() }

		if msgText == ".id" {
			fmt.Printf("📩 [Command] .id from %s\n", v.Info.Chat)
			client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
				Conversation: proto.String(fmt.Sprintf("📍 *Chat ID:* `%s`", v.Info.Chat)),
			})
		} else if msgText == ".chk" || msgText == ".check" {
			sendTestButtons(client, v.Info.Chat)
		}
	}
}

func main() {
	fmt.Println("🚀 [System] Starting Kami OTP Bot with Button Lab...")
	
	dbLog := waLog.Stdout("Database", "INFO", true)
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:kami_bot.db?_foreign_keys=on", dbLog)
	if err != nil { panic(err) }
	
	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil { panic(err) }

	client = whatsmeow.NewClient(deviceStore, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(eventHandler)

	if client.Store.ID == nil {
		err = client.Connect()
		if err != nil { panic(err) }
		fmt.Println("⏳ [Pairing] Requesting Pairing Code...")
		code, err := client.PairPhone(context.Background(), Config.OwnerNumber, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
		if err != nil {
			fmt.Printf("❌ [Error] %v\n", err)
			return
		}
		fmt.Printf("\n🔑 PAIRING CODE: %s\n\n", code)
	} else {
		err = client.Connect()
		if err != nil { panic(err) }
		fmt.Println("✅ [System] Connected!")
		go func() {
			for {
				checkOTPs(client)
				time.Sleep(time.Duration(Config.Interval) * time.Second)
			}
		}()
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	client.Disconnect()
}