package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	
	"github.com/gorilla/websocket"
	
	"github.com/danielpaulus/go-ios/ios"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex
)

func HandleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer ws.Close()
	
	clientsMu.Lock()
	clients[ws] = true
	clientsMu.Unlock()
	
	log.Println("Client connected via WebSocket")
	
	// Send current device list immediately
	sendDeviceList(ws)
	
	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			log.Printf("Client disconnected: %v", err)
			clientsMu.Lock()
			delete(clients, ws)
			clientsMu.Unlock()
			break
		}
		
		log.Printf("Received: %s", message)
		
		// Handle commands
		var cmd map[string]interface{}
		if err := json.Unmarshal(message, &cmd); err == nil {
			handleCommand(ws, cmd)
		}
	}
}

func sendDeviceList(ws *websocket.Conn) {
	deviceList, err := ios.ListDevices()
	if err != nil {
		ws.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": "Failed to list devices: " + err.Error(),
		})
		return
	}
	
	// Convert to simple format for web
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
	
	ws.WriteJSON(map[string]interface{}{
		"type": "devices",
		"data": simpleDevices,
	})
}

func handleCommand(ws *websocket.Conn, cmd map[string]interface{}) {
	command, ok := cmd["command"].(string)
	if !ok {
		ws.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": "Invalid command format",
		})
		return
	}
	
	switch command {
	case "refreshDevices":
		sendDeviceList(ws)
	case "connect":
		// TODO: Connect to specific device
		ws.WriteJSON(map[string]interface{}{
			"type": "info",
			"data": "Connect feature coming soon",
		})
	default:
		ws.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": fmt.Sprintf("Unknown command: %s", command),
		})
	}
}

// BroadcastToAll sends message to all connected clients
func BroadcastToAll(msgType string, data interface{}) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	
	for client := range clients {
		client.WriteJSON(map[string]interface{}{
			"type": msgType,
			"data": data,
		})
	}
}
