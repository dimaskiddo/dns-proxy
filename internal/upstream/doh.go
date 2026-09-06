package upstream

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/miekg/dns"
)

// ForwardDoH sends m as a DNS-over-HTTPS POST to each URL in turn, returning
// the first successful response.
func (c *Client) ForwardDoH(m *dns.Msg) (*dns.Msg, error) {
	var errLast error

	packed, err := m.Pack()
	if err != nil {
		return nil, fmt.Errorf("[DOH] failed to pack query: %w", err)
	}

	for _, url := range c.DoHURLs {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.Timeout)*time.Second)

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(packed))
		if err != nil {
			cancel()
			return nil, err
		}

		req.Header.Set("Content-Type", "application/dns-message")
		req.Header.Set("Accept", "application/dns-message")

		resp, err := c.dohClient.Do(req)
		if err != nil {
			errLast = err
			cancel()

			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			errLast = fmt.Errorf("DoH upstream %s returned %d", url, resp.StatusCode)
			cancel()

			continue
		}

		bufPtr := c.bufPool.Get().(*[]byte)

		buf := (*bufPtr)[:0]
		buffer := bytes.NewBuffer(buf)

		_, err = buffer.ReadFrom(resp.Body)
		resp.Body.Close()

		if err != nil {
			c.bufPool.Put(bufPtr)
			errLast = err
			cancel()

			continue
		}

		msg := new(dns.Msg)
		err = msg.Unpack(buffer.Bytes())

		c.bufPool.Put(bufPtr)
		cancel()

		if err != nil {
			errLast = err
			continue
		}

		return msg, nil
	}

	return nil, fmt.Errorf("[DOH] failed to dial DNS upstreams: %w", errLast)
}
