package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/miekg/dns"
)

func forwardDoH(m *dns.Msg, urls []string) (*dns.Msg, error) {
	var errLast error

	packed, _ := m.Pack()
	encoded := base64.RawURLEncoding.EncodeToString(packed)

	for _, url := range urls {
		dohURL := fmt.Sprintf("%s?dns=%s", url, encoded)
		req, err := http.NewRequest("GET", dohURL, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Accept", "application/dns-message")

		resp, err := dohClient.Do(req)
		if err != nil {
			errLast = err
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			errLast = fmt.Errorf("Error DoH Upstream %s Returned %d", url, resp.StatusCode)
			continue
		}

		bufPtr := bufPool.Get().(*[]byte)

		buf := (*bufPtr)[:0]
		buffer := bytes.NewBuffer(buf)

		_, err = buffer.ReadFrom(resp.Body)
		resp.Body.Close()

		if err != nil {
			// Return Buffer to Pool when Error Occurred
			bufPool.Put(bufPtr)
			errLast = err
			continue
		}

		msg := new(dns.Msg)
		err = msg.Unpack(buffer.Bytes())

		// Return Buffer to Pool
		bufPool.Put(bufPtr)

		if err != nil {
			errLast = err
			continue
		}

		return msg, nil
	}

	return nil, fmt.Errorf("[DOH] Error Failed to Dial DNS Upstreams: %v", errLast)
}
