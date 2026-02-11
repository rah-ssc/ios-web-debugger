package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
	
	"github.com/yourname/ios-debug-companion/internal/devices"
	"github.com/yourname/ios-debug-companion/pkg/websocket"
	
	"github.com/danielpaulus/go-ios/ios"
)

func main() {
	fmt.Println("=== iOS Web Debug Companion ===")
	fmt.Println("Version: 0.1.0")
	fmt.Println("Starting WebSocket server on :9223")
	
	// Start device monitoring in background
	go devices.StartMonitoring()
	
	// Start device change broadcaster
	go broadcastDeviceChanges()
	
	// WebSocket endpoint
	http.HandleFunc("/ws", websocket.HandleConnections)
	
	// Status endpoint
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"running","version":"0.1.0"}`))
	})
	
	// Device list endpoint (UPDATED - returns real data)
	http.HandleFunc("/devices", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		deviceList, err := ios.ListDevices()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": err.Error(),
				"devices": []interface{}{},
			})
			return
		}
		
		// Convert to simple format
		var simpleDevices []map[string]interface{}
		for _, entry := range deviceList.DeviceList {
			device := map[string]interface{}{
				"serial":      entry.Properties.SerialNumber,
				"connection":  entry.Properties.ConnectionType,
				"deviceId":    entry.DeviceID,
				"productId":   entry.Properties.ProductID,
				"messageType": entry.MessageType,
			}
			simpleDevices = append(simpleDevices, device)
		}
		
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count":   len(simpleDevices),
			"devices": simpleDevices,
		})
	})
	
	// Serve test client
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "test-client.html")
	})
	
	// Start server
	log.Println("Server starting...")
	log.Println("1. Web UI: http://localhost:9223")
	log.Println("2. WebSocket: ws://localhost:9223/ws")
	log.Println("3. Status: http://localhost:9223/status")
	log.Println("4. Devices API: http://localhost:9223/devices")
	
	if err := http.ListenAndServe(":9223", nil); err != nil {
		log.Fatal("Server failed:", err)
	}
}

func broadcastDeviceChanges() {
	// Broadcast device list changes every 15 seconds
	for {
		time.Sleep(15 * time.Second)
		websocket.BroadcastToAll("ping", "Devices updated")
	}
}
