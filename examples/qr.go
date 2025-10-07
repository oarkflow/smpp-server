package main

import (
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/skip2/go-qrcode"
)

// ---------------------- Event Broker ----------------------
type EventBroker struct {
	mu      sync.Mutex
	clients map[chan string]bool
}

func newEventBroker() *EventBroker {
	return &EventBroker{
		clients: make(map[chan string]bool),
	}
}

func (b *EventBroker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	msgChan := make(chan string)
	b.mu.Lock()
	b.clients[msgChan] = true
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.clients, msgChan)
		b.mu.Unlock()
		close(msgChan)
	}()

	for msg := range msgChan {
		fmt.Fprintf(w, "data: %s\n\n", msg)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}

func (b *EventBroker) Broadcast(msg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for c := range b.clients {
		select {
		case c <- msg:
		default:
		}
	}
}

var broker = newEventBroker()
var lanIP string

// ---------------------- Main ----------------------
func main() {
	ip, err := getLocalIP()
	if err != nil {
		log.Fatalf("❌ Could not detect LAN IP: %v", err)
	}
	lanIP = ip

	_ = os.MkdirAll("qr", 0755)

	http.Handle("/events", broker)
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/scan", handleScan)
	http.HandleFunc("/pay", handlePay)
	http.Handle("/qr/", http.StripPrefix("/qr/", http.FileServer(http.Dir("./qr"))))

	fmt.Printf("🚀 Server started!\n")
	fmt.Printf("📱 Open on desktop:  http://localhost:8080\n")
	fmt.Printf("📲 Scan from phone:  http://%s:8080\n", lanIP)

	log.Fatal(http.ListenAndServe(":8080", nil))
}

// ---------------------- Handlers ----------------------
func handleIndex(w http.ResponseWriter, r *http.Request) {
	id := uuid.New().String()
	amount := 1200 // NPR
	scanURL := fmt.Sprintf("http://%s:8080/scan?id=%s", lanIP, id)

	// Generate QR with scan URL only
	qrFile := fmt.Sprintf("qr/%s.png", id)
	if err := qrcode.WriteFile(scanURL, qrcode.Medium, 256, qrFile); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	tmpl := `
	<!DOCTYPE html>
	<html>
	<head>
		<meta charset="utf-8"/>
		<title>Merchant QR Payment Demo</title>
		<style>
			body { font-family: Arial; text-align:center; margin-top:50px; }
			img { border:3px solid #ccc; border-radius:10px; padding:5px; }
			#status { margin-top:20px; font-size:20px; color:#555; transition: all 0.4s; }
			#receipt { margin-top:20px; font-size:16px; color:#333; white-space: pre-line; }
		</style>
	</head>
	<body>
		<h1>Merchant: Global IME Bank Ltd</h1>
		<p><strong>Amount:</strong> NPR {{.Amount}}</p>
		<img src="/{{.QRPath}}" width="256" height="256" alt="QR">
		<div id="status">Waiting for scan...</div>
		<div id="receipt"></div>

		<script>
			const evtSource = new EventSource("/events");
			evtSource.onmessage = function(e){
				if(e.data.includes("{{.ID}} scanned")){
					document.getElementById("status").innerText = "✅ QR Scanned!";
					document.getElementById("status").style.color = "orange";
				}
				if(e.data.includes("{{.ID}} paid")){
					document.getElementById("status").innerText = "💰 Payment Confirmed!";
					document.getElementById("status").style.color = "green";
					document.getElementById("receipt").innerText = e.data.split("||")[1];
				}
			};
		</script>
	</body>
	</html>
	`

	t, _ := template.New("page").Parse(tmpl)
	t.Execute(w, map[string]interface{}{
		"ID":     id,
		"QRPath": qrFile,
		"Amount": amount,
	})
}

func handleScan(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing id", 400)
		return
	}

	// Broadcast QR scanned
	broker.Broadcast(fmt.Sprintf("%s scanned", id))

	// Simulated mobile banking page
	fmt.Fprintf(w, `
	<!DOCTYPE html>
	<html>
	<head>
		<meta charset="utf-8"/>
		<title>Mobile Banking App</title>
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
	</head>
	<body style="font-family:Arial; text-align:center; margin-top:50px;">
		<h2>Mobile Banking App</h2>
		<p>Merchant: Global IME Bank Ltd</p>
		<p>Amount: NPR 1200</p>
		<a href="/pay?id=%s" style="font-size:20px; padding:10px 20px; background:#28a745; color:white; text-decoration:none; border-radius:5px;">Pay NPR 1200</a>
	</body>
	</html>
	`, id)
}

func handlePay(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing id", 400)
		return
	}

	// Simulate payment receipt
	receipt := fmt.Sprintf("Transaction ID: %s\nAmount: NPR 1200\nStatus: SUCCESS\nBank Ref: %s", id, uuid.New().String())
	broker.Broadcast(fmt.Sprintf("%s paid||%s", id, receipt))

	fmt.Fprintf(w, `
	<!DOCTYPE html>
	<html>
	<body style="font-family:Arial; text-align:center; margin-top:50px;">
		<h2>💰 Payment Successful!</h2>
		<pre>%s</pre>
		<p>Return to merchant screen.</p>
	</body>
	</html>
	`, receipt)
}

// ---------------------- Utilities ----------------------
func getLocalIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil {
				return ip.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no active network interface found")
}
