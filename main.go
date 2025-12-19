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

func extractOTP(msg string) string {
	re := regexp.MustCompile(`\b\d{3,4}[-\s]?\d{3,4}\b|\b\d{4,8}\b`)
	return re.FindString(msg)
}

func maskNumber(num string) string {
	if len(num) < 7 { return num }
	return num[:5] + "XXXX" + num[len(num)-2:]
}

// --- اے پی آئی چیک کرنے کا فنکشن ---
func checkOTPs(cli *whatsmeow.Client) {
	// اگر کلائنٹ ابھی کنیکٹ نہیں ہوا تو انتظار کریں
	if cli == nil || !cli.IsConnected() {
		fmt.Println("⏳ [Wait] Client not connected yet, skipping this cycle...")
		return
	}

	fmt.Println("🔍 [Monitor] Starting API check cycle...")
	
	for _, url := range Config.OTPApiURLs {
		fmt.Printf("🌐 [Request] Calling: %s\n", url)
		
		httpClient := &http.Client{Timeout: 15 * time.Second}
		resp, err := httpClient.Get(url)
		if err != nil {
			fmt.Printf("⚠️ [SKIP] API error for %s: %v\n", url, err)
			continue 
		}

		var data map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()

		if err != nil {
			fmt.Printf("⚠️ [SKIP] Invalid JSON from %s\n", url)
			continue
		}

		aaData, ok := data["aaData"].([]interface{})
		if !ok {
			fmt.Printf("⚠️ [SKIP] No data in %s\n", url)
			continue
		}

		apiName := "Server-1"
		if strings.Contains(url, "kamibroken") { apiName = "Kami-Broken" }

		for _, row := range aaData {
			r, ok := row.([]interface{})
			if !ok || len(r) < 5 { continue }

			msgID := fmt.Sprintf("%v_%v", r[2], r[0])
			if !lastProcessedIDs[msgID] {
				fmt.Printf("📩 [New OTP] Found message from %s for %v\n", apiName, r[2])
				
				rawTime, _ := r[0].(string)
				countryInfo, _ := r[1].(string)
				phone, _ := r[2].(string)
				service, _ := r[3].(string)
				fullMsg, _ := r[4].(string)

				cFlag, countryWithFlag := GetCountryWithFlag(countryInfo)
				otpCode := extractOTP(fullMsg)

				messageBody := fmt.Sprintf(`✨ *%s | %s Message*⚡
> ⏰ Time: _%s_
> 🌍 Country: _%s_
> 📞 Number: _%s_
> ⚙️ Service: _%s_
> 🔑 OTP: *%s*
> 📡 API: *%s*

📩 Full Msg:
"%s"

_Developed by Nothing Is Impossible_`, cFlag, strings.ToUpper(service), rawTime, countryWithFlag, maskNumber(phone), service, otpCode, apiName, fullMsg)

				// چینلز پر سینڈ کرنا
				for _, jidStr := range Config.OTPChannelIDs {
					jid, err := types.ParseJID(jidStr)
					if err != nil {
						fmt.Printf("❌ [JID Error] Invalid ID %s: %v\n", jidStr, err)
						continue
					}
					
					fmt.Printf("📤 [Sending] Attempting to send to: %s\n", jidStr)
					_, err = cli.SendMessage(context.Background(), jid, &waProto.Message{
						Conversation: proto.String(strings.TrimSpace(messageBody)),
					})
					if err != nil {
						fmt.Printf("❌ [Send Failed] Channel %s: %v\n", jidStr, err)
					} else {
						fmt.Printf("✅ [Success] OTP forwarded to %s\n", jidStr)
					}
				}
				lastProcessedIDs[msgID] = true
			}
		}
	}
}

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		msgText := v.Message.GetConversation()
		if msgText == "" { msgText = v.Message.GetExtendedTextMessage().GetText() }

		if msgText == ".id" {
			client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
				Conversation: proto.String(fmt.Sprintf("📍 Chat ID: `%s`", v.Info.Chat)),
			})
		}
	}
}

func main() {
	fmt.Println("🚀 [System] Initializing Kami OTP Bot...")
	
	dbLog := waLog.Stdout("Database", "INFO", true)
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:kami_bot.db?_foreign_keys=on", dbLog)
	if err != nil { panic(err) }
	
	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil { panic(err) }

	client = whatsmeow.NewClient(deviceStore, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(eventHandler)

	// ہمیشہ کنیکٹ کریں
	err = client.Connect()
	if err != nil { panic(err) }

	// اگر ڈیوائس رجسٹر نہیں ہے تو پیرنگ کوڈ دکھائیں
	if client.Store.ID == nil {
		fmt.Println("⏳ [Auth] New session. Requesting Pairing Code...")
		time.Sleep(3 * time.Second)
		code, err := client.PairPhone(context.Background(), Config.OwnerNumber, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
		if err != nil { fmt.Printf("❌ [Error] %v\n", err); return }
		fmt.Printf("\n🔑 PAIRING CODE: %s\n\n", code)
	} else {
		fmt.Println("✅ [System] Existing session found. Logged in!")
	}

	// --- مانیٹرنگ لوپ (اب یہ ہر حال میں چلے گا) ---
	go func() {
		fmt.Println("⏰ [Scheduler] Monitoring loop started.")
		for {
			// صرف تب مانیٹر کریں جب لاگ ان ہو
			if client.IsLoggedIn() {
				checkOTPs(client)
			} else {
				fmt.Println("😴 [Status] Waiting for login to start monitoring...")
			}
			time.Sleep(time.Duration(Config.Interval) * time.Second)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	fmt.Println("👋 [Shutting Down] Goodbye!")
	client.Disconnect()
}