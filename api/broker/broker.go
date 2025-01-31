package broker

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/global"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

type (
	environment interface {
		global.Logging
		ListenToVids(fun func(vid *vertex.WrappedTx))
	}

	broker struct {
		*mqtt.Server
		environment
	}
)

func Run(addr string, env environment) {

	srv := &broker{
		Server: mqtt.New(&mqtt.Options{
			InlineClient: true, // this needs to be set to true to allow the broker to send messages itselfil),
		}),
		environment: env,
	}
	_ = srv.Server.AddHook(new(auth.AllowHook), nil)

	ws := listeners.NewWebsocket(listeners.Config{
		ID:        "ws1",
		Address:   ":8082",
		TLSConfig: nil})

	if err := srv.Server.AddListener(ws); err != nil {
		log.Fatal("adding websocket listener failed: %w", err)
	}

	// Start the MQTT server
	go func() {
		if err := srv.Server.Serve(); err != nil {
			log.Fatal("MQTT server error:", err)
		}
	}()
	log.Println("MQTT server running on tcp://localhost:1883 and ws://localhost:8082")

	// Start publishing messages
	//go srv.startPublishing()

	env.ListenToVids(func(vid *vertex.WrappedTx) {
		env.Tracef("brk", "TX ID: %s", vid.IDShortString())

		log.Println("got vid TXID: ", vid.IDShortString())

		// JSON-encode the vid object
		vidJSON, err := json.Marshal(vid)
		if err != nil {
			log.Printf("Error encoding vid to JSON: %v", err)
			return
		}

		// Send the JSON-encoded vid
		// Replace this with your actual sending logic
		log.Printf("Sending JSON-encoded vid: %s", string(vidJSON))
		// Example: sendToMQTT(vidJSON)
		srv.Server.Publish("test/topic", []byte(vidJSON), false, 0)
		if err != nil {
			log.Printf("Published message err: %s", err.Error())
		}
	})

	// srv.registerHandlers()
}

// 	//srv.Server.Subscribe(topic, func(cl *mqtt.Client, msg packets.Message) {
// 	srv.Server.Subscribe(topic, func(cl *mqtt.Client, msg mqtt.Message) {
// 		fmt.Printf("Received message on %s: %s\n", msg.Topic(), string(msg.Payload()))
// 	})
// 	log.Printf("Subscribed to topic: %s", topic)
// }

// Subscribe to a topic
func (srv *broker) subscribeToTopic(topic string) {
	srv.Server.Subscribe(topic, 1, func(cl *mqtt.Client, sub packets.Subscription, pk packets.Packet) {
		fmt.Printf("Received message on %s: %s\n", pk.TopicName, string(pk.Payload))
	})
	log.Printf("Subscribed to topic: %s", topic)
}

// Publish messages every 5 seconds
func (srv *broker) startPublishing() {
	for {
		time.Sleep(5 * time.Second)
		msg := "Hello from MQTT server!"
		err := srv.Server.Publish("test/topic", []byte(msg), false, 0)
		log.Printf("Published message: %s", msg)
		if err != nil {
			log.Printf("Published message err: %s", err.Error())
		}
	}
}
