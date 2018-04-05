package main

import (
	"os/signal"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/redhat-nfvpe/service-assurance-poc/amqp"
	"github.com/redhat-nfvpe/service-assurance-poc/config"
	"github.com/redhat-nfvpe/service-assurance-poc/elasticsearch"

	"flag"
	"fmt"
	"log"
	"os"
)

/*************** main routine ***********************/
// eventusage and command-line flags
func eventusage() {
	doc := heredoc.Doc(`
  For running with config file use
	********************* config *********************
	$go run events/main.go -config sa.events.config.json
	**************************************************
	For running with AMQP and Prometheus use following option
	********************* Production *********************
	$go run events/main.go -amqp1EventURL=10.19.110.5:5672/collectd/notify -eshost=http://10.19.110.5:9200
	**************************************************************`)
	fmt.Fprintln(os.Stderr, `Required commandline argument missing`)
	fmt.Fprintln(os.Stdout, doc)
	flag.PrintDefaults()
}

func main() {
	// set flags for parsing options
	flag.Usage = eventusage
	fConfigLocation := flag.String("config", "", "Path to configuration file(optional).if provided ignores all command line options")

	fAMQP1EventURL := flag.String("amqp1EventURL", "", "AMQP1.0 events listener example 127.0.0.1:5672/collectd/notify")
	fElasticHostURL := flag.String("eshost", "", "elasticsearch host http://localhost:9200")
	fRestIndex := flag.Bool("resetIndex", false, "Optional Clean all index before on start (default false)")

	flag.Parse()
	var serverConfig saconfig.EventConfiguration
	if len(*fConfigLocation) > 0 { //load configuration
		serverConfig = saconfig.LoadEventConfig(*fConfigLocation)
	} else {
		serverConfig = saconfig.EventConfiguration{
			AMQP1EventURL:  *fAMQP1EventURL,
			ElasticHostURL: *fElasticHostURL,
			RestIndex:      *fRestIndex,
		}

	}

	if len(serverConfig.AMQP1EventURL) == 0 {
		log.Println("AMQP1 Event URL is required")
		eventusage()
		os.Exit(1)
	}
	if len(serverConfig.ElasticHostURL) == 0 {
		log.Println("Elastic Host URL is required")
		eventusage()
		os.Exit(1)
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			// sig is a ^C, handle it
			log.Printf("caught sig: %+v", sig)
			log.Println("Wait for 2 second to finish processing")
			time.Sleep(2 * time.Second)
			os.Exit(0)
		}
	}()

	log.Printf("Config %#v\n", serverConfig)
	eventsNotifier := make(chan string) // Channel for messages from goroutines to main()
	var amqpEventServer *amqplistener.AMQPServer
	///Metric Listener
	amqpEventsurl := fmt.Sprintf("amqp://%s", serverConfig.AMQP1EventURL)
	log.Printf("Connecting to AMQP1 : %s\n", amqpEventsurl)
	amqpEventServer = amqplistener.NewAMQPServer(amqpEventsurl, true, -1, eventsNotifier)
	log.Printf("Listening.....\n")
	var elasticClient *saelastic.ElasticClient
	log.Printf("Connecting to ElasticSearch : %s\n", serverConfig.ElasticHostURL)
	elasticClient = saelastic.CreateClient(serverConfig.ElasticHostURL, serverConfig.RestIndex)
	log.Println("Ready....")

	for {
		select {
		case event := <-amqpEventServer.GetNotifier():
			//log.Printf("Event occured : %#v\n", event)
			indexName, indexType, err := saelastic.GetIndexNameType(event)
			if err != nil {
				log.Printf("Failed to read event %s type in main %s\n", event, err)
			} else {
				id, err := elasticClient.Create(indexName, indexType, event)
				if err != nil {
					log.Printf("Error creating event %s in elastic search %s\n", event, err)
				} else {
					log.Printf("Document created in elasticsearch for mapping: %s ,type: %s, id :%s\n", string(indexName), string(indexType), id)
				}

			}
			continue
		default:
			//no activity
		}
	}

	//TO DO: to close cache server on keyboard interrupt

}
