package devices

import (
	"fmt"
	"time"
	
	"github.com/danielpaulus/go-ios/ios"
)

func StartMonitoring() {
	fmt.Println("Starting iOS device monitor...")
	
	for {
		deviceList, err := ios.ListDevices()
		if err != nil {
			fmt.Printf("Error listing devices: %v\n", err)
		} else {
			devices := deviceList.DeviceList
			
			// Deduplicate by serial number
			seen := make(map[string]bool)
			var uniqueDevices []ios.DeviceEntry
			
			for _, deviceEntry := range devices {
				serial := deviceEntry.Properties.SerialNumber
				if !seen[serial] {
					seen[serial] = true
					uniqueDevices = append(uniqueDevices, deviceEntry)
				}
			}
			
			fmt.Printf("Found %d iOS device(s)\n", len(uniqueDevices))
			
			for i, deviceEntry := range uniqueDevices {
				fmt.Printf("  %d. Serial: %s\n", 
					i+1, 
					deviceEntry.Properties.SerialNumber)
				fmt.Printf("     Connection: %s, DeviceID: %d\n",
					deviceEntry.Properties.ConnectionType,
					deviceEntry.DeviceID)
			}
		}
		
		fmt.Println("---")
		time.Sleep(10 * time.Second)
	}
}
