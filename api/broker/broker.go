package broker

import (
	"fmt"
	"log"
	"time"

	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

type (
	environment interface {
		global.Logging
		ListenToAccount(account ledger.Accountable, fun func(wOut vertex.WrappedOutput))
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

	// Subscribe to a topic
	//srv.subscribeToTopic("test/topic")

	// Start publishing messages
	go srv.startPublishing()

	account, _ := ledger.AccountableFromSource("a(0x033d48aa6f02b3f37811ae82d9c383855d3d23373cbd28ab94639fdd94a4f02d)")
	env.ListenToAccount(account, func(wOut vertex.WrappedOutput) {
		env.Tracef("brk", "output IN: %s", wOut.IDShortString)

		// ret.mutex.Lock()
		// defer ret.mutex.Unlock()

		// if _, already := ret.outputs[wOut]; already {
		// 	env.Tracef(TraceTag, "repeating output %s", wOut.IDShortString)
		// 	return
		// }
		// // reference it
		// if !ret.checkAndReferenceCandidate(wOut) {
		// 	// failed to reference -> ignore
		// 	return
		// }
		// // new referenced output -> put it into the map
		// nowis := time.Now()
		// ret.outputs[wOut] = nowis
		// ret.lastOutputArrived = nowis
		// ret.outputCount++
		// env.Tracef(TraceTag, "output included into input backlog: %s (total: %d)", wOut.IDShortString, len(ret.outputs))
	})

	// srv.registerHandlers()
	// srv.registerMetrics()

	// err := srv.ListenAndServe()
	// util.AssertNoError(err)
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
