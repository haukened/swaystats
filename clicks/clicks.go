package clicks

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
)

// Click represents a click event fed by swaybar back into stdin.
type Click struct {
	Name      string   `json:"name"`
	Instance  string   `json:"instance,omitempty"`
	Button    int      `json:"button"`
	X         int      `json:"x"`
	Y         int      `json:"y"`
	Modifiers []string `json:"modifiers"`
}

// Read consumes newline-delimited JSON click events, emitting them onto out.
// It drops events if the channel is full to avoid blocking the main loop.
func Read(r io.Reader, out chan<- Click) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		var c Click
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			log.Printf("click parse: %v", err)
			continue
		}
		select {
		case out <- c:
		default:
			// drop if full
		}
	}
	if err := sc.Err(); err != nil {
		log.Printf("click scanner: %v", err)
	}
}
