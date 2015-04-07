package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	ext_ip_url = "http://ipecho.net/plain"
	validIP    = `\b\d+(\.\d+){3}\b`
	responseOK = `good|nochg`
)

var (
	r_ip         *regexp.Regexp
	r_responseOK *regexp.Regexp
)

func init() {
	r_ip = regexp.MustCompile(validIP)
	r_responseOK = regexp.MustCompile(responseOK)
}

func main() {
	fmt.Printf("Starting UpdateIP\n")

	//process1 := newWriterProcessor(os.Stdout)

	process2 := newHttpPoster("https://dns.loopia.se/XDynDNSServer/XDynDNS.php", // url
		"myip",         // ipField
		"hostname",     // hostnameField
		"mydomain.com", // hostname
		"username",     // username
		"password",     // password
	)

	for {
		ip, err := getExtIP()
		if err != nil {
			fmt.Println(err)
		} else {
			//processIP(ip, process1, process2)
			processIP(ip, process2)
		}
		time.Sleep(time.Second * 10)
	}
}

func getExtIP() (string, error) {
	resp, err := http.Get(ext_ip_url)
	if err != nil {
		return "", errors.New(fmt.Sprintf("No response from %s", ext_ip_url))
	}
	defer resp.Body.Close()

	ip, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Could not read response from %s", ext_ip_url))
	}

	v_ip := r_ip.FindString(string(ip))
	if len(v_ip) > 0 {
		return v_ip, nil
	}
	return string(ip), errors.New(fmt.Sprintf("No IP found in response from %s", ext_ip_url))
}

// processIP runs as a separate goroutine. It consumes the given channel and sends it forward to
// processing routines.
func processIP(new_ip string, chans ...chan string) {
	fmt.Printf("\nprocessIP: Got IP %s\n", new_ip)

	for _, processChannel := range chans {
		processChannel <- new_ip
	}
}

func newWriterProcessor(output io.Writer) chan string {
	channel := make(chan string)
	go processToWriter(output, channel)
	return channel
}

func processToWriter(output io.Writer, new_ip_chan <-chan string) {
	lastIP := ""
	for {
		select {
		case new_ip := <-new_ip_chan:
			if lastIP != new_ip {
				fmt.Fprintf(output, "processToWriter: New IP %s\n", new_ip)
				lastIP = new_ip
			}
		}
	}
}

type httpPoster struct {
	ipChan        <-chan string
	url           string
	ipField       string
	hostnameField string
	hostname      string
	username      string
	password      string
	lastIP        string
}

// httpPoster.Process runs in it's own goroutine
func (p *httpPoster) Process() {
	for {
		select {
		case newIP := <-p.ipChan:
			err := p.post(newIP)
			if err != nil {
				fmt.Println(err)
			}
		}
	}
}

func (p *httpPoster) post(newIP string) error {
	if newIP == p.lastIP {
		return nil
	}

	fmt.Printf("httpPoster.post: New IP %s\n", newIP)

	v := url.Values{}
	v.Set(p.ipField, newIP)
	v.Set(p.hostnameField, p.hostname)

	req, err := http.NewRequest("POST", p.url, strings.NewReader(v.Encode()))
	if err != nil {
		return errors.New("httpPoster.post: Could not initialize request")
	}

	req.SetBasicAuth(p.username, p.password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New(fmt.Sprintf("httpPoster.post: Error getting response: %v", err))
	}
	defer resp.Body.Close()

	if !statusOK(resp.StatusCode) {
		return errors.New(fmt.Sprintf("httpPoster.post: Response not OK: %v", err))
	}

	bodyText, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.New(fmt.Sprintf("httpPoster.post: Error getting body: %v", err))
	}
	body := string(bodyText)

	good_response := r_responseOK.MatchString(body)
	if !good_response {
		return errors.New(fmt.Sprintf("httpPoster.post: Response not OK: %s", body))
	}

	p.lastIP = newIP
	fmt.Printf("httpPoster.post: Successfully set IP %s for host %s, response: %s", p.lastIP, p.hostname, body)

	return nil
}

func newHttpPoster(url, ipField, hostnameField, hostname, username, password string) chan string {
	channel := make(chan string)
	poster := &httpPoster{
		ipChan:        channel,
		url:           url,
		ipField:       ipField,
		hostnameField: hostnameField,
		hostname:      hostname,
		username:      username,
		password:      password,
	}

	go poster.Process()
	return channel
}

func statusOK(status int) bool {
	if status < 200 {
		return false
	}
	if status >= 300 {
		return false
	}
	return true
}
