package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/miekg/dns"
)

func forwardDoH(m *dns.Msg, urls []string) (*dns.Msg, error) {
	var errLast error

	packed, _ := m.Pack()

	for _, url := range urls {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Upstream.Timeout)*time.Second)

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(packed))
		if err != nil {
			cancel()
			return nil, err
		}

		req.Header.Set("Content-Type", "application/dns-message")
		req.Header.Set("Accept", "application/dns-message")

		resp, err := dohClient.Do(req)
		if err != nil {
			errLast = err
			cancel()

			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			errLast = fmt.Errorf("Error DoH Upstream %s Returned %d", url, resp.StatusCode)
			cancel()

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
			cancel()

			continue
		}

		msg := new(dns.Msg)
		err = msg.Unpack(buffer.Bytes())

		// Return Buffer to Pool
		bufPool.Put(bufPtr)
		cancel()

		if err != nil {
			errLast = err
			continue
		}

		return msg, nil
	}

	return nil, fmt.Errorf("[DOH] Error Failed to Dial DNS Upstreams: %v", errLast)
}
