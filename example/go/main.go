package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"git.apache.org/thrift.git/lib/go/thrift"
	"github.com/Workiva/frugal-go"
	"github.com/nats-io/nats"

	"github.com/Workiva/frugal/example/go/gen-go/event"
)

func Usage() {
	fmt.Fprint(os.Stderr, "Usage of ", os.Args[0], ":\n")
	flag.PrintDefaults()
	fmt.Fprint(os.Stderr, "\n")
}

func main() {
	flag.Usage = Usage
	var (
		client   = flag.Bool("client", false, "Run client")
		server   = flag.Bool("server", false, "Run server")
		pub      = flag.Bool("pub", false, "Run publisher")
		sub      = flag.Bool("sub", false, "Run subscriber")
		protocol = flag.String("P", "binary", "Specify the protocol (binary, compact, json, simplejson)")
		framed   = flag.Bool("framed", false, "Use framed transport")
		buffered = flag.Bool("buffered", false, "Use buffered transport")
		addr     = flag.String("addr", "", "NATS address")
		secure   = flag.Bool("secure", false, "Use tls secure transport")
	)
	flag.Parse()

	fprotocolFactory := frugal.NewFBinaryProtocolFactoryDefault()
	var tprotocolFactory thrift.TProtocolFactory
	switch *protocol {
	case "compact":
		tprotocolFactory = thrift.NewTCompactProtocolFactory()
	case "simplejson":
		tprotocolFactory = thrift.NewTSimpleJSONProtocolFactory()
	case "json":
		tprotocolFactory = thrift.NewTJSONProtocolFactory()
	case "binary", "":
		tprotocolFactory = thrift.NewTBinaryProtocolFactoryDefault()
	default:
		fmt.Fprint(os.Stderr, "Invalid protocol specified", protocol, "\n")
		Usage()
		os.Exit(1)
	}

	var transportFactory thrift.TTransportFactory
	if *buffered {
		transportFactory = thrift.NewTBufferedTransportFactory(8192)
	} else {
		transportFactory = thrift.NewTTransportFactory()
	}

	if *framed {
		transportFactory = thrift.NewTFramedTransportFactory(transportFactory)
	}

	_addr := *addr
	if *client || *server {
		if _addr == "" {
			_addr = "localhost:9090"
		}
		if *client {
			if err := runClient(transportFactory, fprotocolFactory, _addr, *secure); err != nil {
				fmt.Println("error running client:", err)
			}
		} else if *server {
			if err := runServer(transportFactory, fprotocolFactory, _addr, *secure); err != nil {
				fmt.Println("error running server:", err)
			}
		}
		return
	}

	if _addr == "" {
		_addr = nats.DefaultURL
	}
	natsOptions := nats.DefaultOptions
	natsOptions.Servers = []string{_addr}
	natsOptions.Secure = *secure
	conn, err := natsOptions.Connect()
	if err != nil {
		panic(err)
	}

	if *sub {
		if err := runSubscriber(conn, tprotocolFactory, transportFactory); err != nil {
			fmt.Println("error running subscriber:", err)
		}
	} else if *pub {
		if err := runPublisher(conn, tprotocolFactory, transportFactory); err != nil {
			fmt.Println("error running publisher:", err)
		}
	}
}

// Client handler
func handleClient(client *event.FrugalFooClient) (err error) {
	ctx := frugal.NewContext("")
	result, err := client.Blah(ctx, 100)
	fmt.Printf("Blah = %d\n", result)
	fmt.Println(err)
	fmt.Println(ctx.ResponseHeader("foo"))
	fmt.Printf("%+v\n", ctx)
	return err
}

// Client runner
func runClient(transportFactory thrift.TTransportFactory, protocolFactory frugal.FProtocolFactory, addr string, secure bool) error {
	var transport thrift.TTransport
	var err error
	if secure {
		cfg := new(tls.Config)
		cfg.InsecureSkipVerify = true
		transport, err = thrift.NewTSSLSocket(addr, cfg)
	} else {
		transport, err = thrift.NewTSocket(addr)
	}
	if err != nil {
		fmt.Println("Error opening socket:", err)
		return err
	}
	transport = transportFactory.GetTransport(transport)
	defer transport.Close()
	if err := transport.Open(); err != nil {
		return err
	}
	return handleClient(event.NewFrugalFooClientFactory(transport, protocolFactory))
}

// Sever handler
type FooHandler struct {
}

func (f *FooHandler) Ping(ctx frugal.Context) error {
	fmt.Printf("Ping(%s)\n", ctx)
	return nil
}

func (f *FooHandler) Blah(ctx frugal.Context, num int32) (int64, error) {
	fmt.Printf("Blah(%s, %d)\n", ctx, num)
	ctx.AddResponseHeader("foo", "bar")
	return 42, nil
}

// Server runner
func runServer(transportFactory thrift.TTransportFactory,
	protocolFactory frugal.FProtocolFactory, addr string, secure bool) error {
	var transport thrift.TServerTransport
	var err error
	if secure {
		cfg := new(tls.Config)
		if cert, err := tls.LoadX509KeyPair("server.crt", "server.key"); err == nil {
			cfg.Certificates = append(cfg.Certificates, cert)
		} else {
			return err
		}
		transport, err = thrift.NewTSSLServerSocket(addr, cfg)
	} else {
		transport, err = thrift.NewTServerSocket(addr)
	}

	if err != nil {
		return err
	}
	fmt.Printf("%T\n", transport)
	handler := &FooHandler{}
	processor := event.NewFrugalFooProcessor(handler)
	server := frugal.NewFSimpleServer4(processor, transport, transportFactory, protocolFactory)

	fmt.Println("Starting the simple server... on ", addr)
	return server.Serve()
}

// Subscriber runner
func runSubscriber(conn *nats.Conn, protocolFactory thrift.TProtocolFactory,
	transportFactory thrift.TTransportFactory) error {
	factory := frugal.NewNATSTransportFactory(conn)
	provider := frugal.NewProvider(factory, transportFactory, protocolFactory)
	subscriber := event.NewEventsSubscriber(provider)
	_, err := subscriber.SubscribeEventCreated("foouser", func(e *event.Event) {
		fmt.Printf("received %+v\n", e)
	})
	if err != nil {
		return err
	}
	ch := make(chan bool)
	log.Println("Subscriber started...")
	<-ch
	return nil
}

// Publisher runner
func runPublisher(conn *nats.Conn, protocolFactory thrift.TProtocolFactory,
	transportFactory thrift.TTransportFactory) error {
	factory := frugal.NewNATSTransportFactory(conn)
	provider := frugal.NewProvider(factory, transportFactory, protocolFactory)
	publisher := event.NewEventsPublisher(provider)
	event := &event.Event{Message: "hello, world!"}
	if err := publisher.PublishEventCreated("foouser", event); err != nil {
		return err
	}
	fmt.Println("EventCreated()")
	time.Sleep(time.Second)
	return nil
}
