package websockets

import (
	"crypto/tls"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

type CustomDialer struct {
	Dialer  websocket.Dialer
	URL     url.URL
	Headers http.Header
}

func NewCustomDialer(urlStr, token string, insecureSkipVerify bool) *CustomDialer {
	u := url.URL{}
	parsedUrl, err := u.Parse(urlStr)
	if err != nil {
		log.Fatalf("Failed to parse URL: %v", err)
	}
	headers := http.Header{}
	headers.Add("Cookie", "R_SESS"+"="+token)

	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify},
		Subprotocols:    []string{"base64.channel.k8s.io"},
	}

	return &CustomDialer{
		Dialer:  dialer,
		URL:     *parsedUrl,
		Headers: headers,
	}
}

func (c *CustomDialer) Connect() (*websocket.Conn, *http.Response, error) {
	return c.Dialer.Dial(c.URL.String(), c.Headers)
}
