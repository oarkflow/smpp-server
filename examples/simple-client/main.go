package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oarkflow/smpp-server/pkg/smpp"
)

func main() {
	fmt.Println("Starting Simple SMPP Client...")

	// Create client with default options
	client, err := smpp.New()
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	// Set up delivery report callback
	client.OnReport(func(report *smpp.DeliveryReport) {
		fmt.Printf("📨 Delivery Report: ID=%s, Status=%s, Timestamp=%s\n",
			report.MessageID, report.Status, report.Timestamp.Format("2006-01-02 15:04:05"))
	})

	// Connect to server
	ctx := context.Background()
	fmt.Println("Connecting to SMPP server...")
	if err := client.Connect(ctx); err != nil {
		log.Fatal("Failed to connect:", err)
	}
	fmt.Println("✅ Connected and bound to SMPP server")

	// Send a simple SMS
	fmt.Println("Sending test SMS...")
	err = client.SendSMS("12345", "Hello from Simple SMPP Client!", "67890")
	if err != nil {
		log.Printf("Failed to send SMS: %v", err)
	} else {
		fmt.Println("✅ SMS sent successfully")
	}

	// Wait a bit
	time.Sleep(2 * time.Second)

	// Send another SMS with options
	fmt.Println("Sending SMS with options...")
	options := &smpp.SMSOptions{
		From:    "54321",
		To:      "09876",
		Message: "This is a test message sent with options!",
	}
	err = client.SendSMSWithOptions(options)
	if err != nil {
		log.Printf("Failed to send SMS with options: %v", err)
	} else {
		fmt.Println("✅ SMS with options sent successfully")
	}

	// Wait for delivery reports
	fmt.Println("Waiting for delivery reports...")
	time.Sleep(5 * time.Second)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	fmt.Println("Press Ctrl+C to exit...")

	<-sigChan
	fmt.Println("Shutting down...")

	// Disconnect
	if err := client.Disconnect(ctx); err != nil {
		log.Printf("Error during disconnect: %v", err)
	} else {
		fmt.Println("✅ Disconnected gracefully")
	}
}
