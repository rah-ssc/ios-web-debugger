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
			
			fmt.Printf("Found %d iOS device(s)\n", len(devices))
			
			for i, deviceEntry := range devices {
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
