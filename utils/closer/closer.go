package closer

import (
	"io"
	"log"
)

// Close attempts to close all provided resources, logging any errors without halting execution.
func Close(closers ...io.Closer) {
	for _, c := range closers {
		err := c.Close()
		if err != nil {
			log.Println("error closing resource:", err.Error())
		}
	}
}
