package bambu

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Client struct {
	host       string
	accessCode string
	serial     string
	onUpdate   func(PrinterState)
	onLog      func(string)

	mu       sync.Mutex
	client   mqtt.Client
	rawState map[string]any
	hms      *HMSDictionary
	connected bool
}

func NewClient(host, accessCode, serial string, onUpdate func(PrinterState), onLog func(string)) *Client {
	return &Client{
		host:       host,
		accessCode: accessCode,
		serial:     serial,
		onUpdate:   onUpdate,
		onLog:      onLog,
		rawState:   make(map[string]any),
		hms:        LoadOrDownloadHMS(onLog),
	}
}

func (c *Client) Connect() error {
	c.log(fmt.Sprintf("Connecting to Bambu Lab printer at %s:8883...", c.host))

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tls://%s:8883", c.host))
	opts.SetClientID(fmt.Sprintf("BambuPresence-Go-%d", time.Now().UnixNano()%100000))
	opts.SetUsername("bblp")
	opts.SetPassword(c.accessCode)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetConnectTimeout(10 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetTLSConfig(&tls.Config{
		InsecureSkipVerify: true,
	})

	opts.SetOnConnectHandler(func(cl mqtt.Client) {
		c.mu.Lock()
		c.connected = true
		c.mu.Unlock()
		c.log("Connected to printer MQTT broker.")

		topic := fmt.Sprintf("device/%s/report", c.serial)
		token := cl.Subscribe(topic, 0, c.handleMessage)
		if token.WaitTimeout(5*time.Second) && token.Error() != nil {
			c.log(fmt.Sprintf("Failed to subscribe to %s: %v", topic, token.Error()))
			return
		}
		c.log(fmt.Sprintf("Subscribed to %s", topic))
		c.RequestPushAll()
	})

	opts.SetConnectionLostHandler(func(cl mqtt.Client, err error) {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		c.log(fmt.Sprintf("Printer connection lost: %v (reconnecting...)", err))
	})

	cl := mqtt.NewClient(opts)
	c.mu.Lock()
	c.client = cl
	c.mu.Unlock()

	token := cl.Connect()
	go func() {
		if !token.WaitTimeout(12 * time.Second) {
			c.log("MQTT connection timed out.")
			return
		}
		if err := token.Error(); err != nil {
			c.log(fmt.Sprintf("MQTT connection error: %v", err))
		}
	}()

	return nil
}

func (c *Client) Disconnect() {
	c.mu.Lock()
	cl := c.client
	c.client = nil
	c.connected = false
	c.mu.Unlock()

	if cl != nil && cl.IsConnected() {
		c.log("Disconnecting from printer...")
		cl.Disconnect(500)
	}
}

func (c *Client) RequestPushAll() {
	c.mu.Lock()
	cl := c.client
	c.mu.Unlock()
	if cl == nil || !cl.IsConnected() {
		return
	}

	req := map[string]any{
		"pushing": map[string]any{
			"sequence_id": "0",
			"command":     "pushall",
		},
	}
	payload, _ := json.Marshal(req)
	topic := fmt.Sprintf("device/%s/request", c.serial)
	cl.Publish(topic, 0, false, payload)
}

func (c *Client) handleMessage(_ mqtt.Client, msg mqtt.Message) {
	var payload map[string]any
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		c.log(fmt.Sprintf("Error decoding MQTT payload: %v", err))
		return
	}

	printData, ok := payload["print"].(map[string]any)
	if !ok {
		return
	}

	c.mu.Lock()
	mergeMap(c.rawState, printData)
	state := ParseState(c.rawState, c.serial, c.hms, c.connected)
	c.mu.Unlock()

	if c.onUpdate != nil {
		c.onUpdate(state)
	}
}

func (c *Client) log(msg string) {
	if c.onLog != nil {
		c.onLog(msg)
	}
}

func mergeMap(dst, src map[string]any) {
	for k, v := range src {
		if srcMap, ok := v.(map[string]any); ok {
			if dstMap, exists := dst[k].(map[string]any); exists {
				mergeMap(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
}
