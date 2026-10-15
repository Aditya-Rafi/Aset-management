package httpx

import "log"

func logErr(err error) {
	if err != nil {
		log.Printf("ERROR: %v", err)
	}
}
