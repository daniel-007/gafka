package command

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Shopify/sarama"
	"github.com/funkygao/gafka/cmd/kateway/manager"
	"github.com/funkygao/gafka/ctx"
	"github.com/funkygao/gafka/zk"
	"github.com/funkygao/gocli"
	"github.com/funkygao/golib/color"
	"github.com/go-ozzo/ozzo-dbx"
	_ "github.com/go-sql-driver/mysql"
	"github.com/olekukonko/tablewriter"
)

type TopicInfo struct {
	AppId          string `db:"AppId"`
	TopicName      string `db:"TopicName"`
	TopicIntro     string `db:"TopicIntro"`
	KafkaTopicName string `db:"KafkaTopicName"`
}

type Verify struct {
	Ui  cli.Ui
	Cmd string

	zone     string
	cluster  string
	interval time.Duration

	mode     string
	mysqlDsn string

	topics []TopicInfo

	kafkaTopics map[string]string        // topic:cluster
	kfkClients  map[string]sarama.Client // cluster:client
	psubClient  map[string]sarama.Client // cluster:client
	zkzone      *zk.ZkZone
}

func (this *Verify) Run(args []string) (exitCode int) {
	cmdFlags := flag.NewFlagSet("verify", flag.ContinueOnError)
	cmdFlags.Usage = func() { this.Ui.Output(this.Help()) }
	cmdFlags.StringVar(&this.zone, "z", ctx.ZkDefaultZone(), "")
	cmdFlags.StringVar(&this.cluster, "c", "bigtopic", "")
	cmdFlags.StringVar(&this.mode, "mode", "p", "")
	cmdFlags.DurationVar(&this.interval, "i", time.Second*10, "")
	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	if validateArgs(this, this.Ui).
		require("-z").
		invalid(args) {
		return 2
	}

	ensureZoneValid(this.zone)
	this.zkzone = zk.NewZkZone(zk.DefaultConfig(this.zone, ctx.ZoneZkAddrs(this.zone)))
	this.kafkaTopics = make(map[string]string)
	this.kfkClients = make(map[string]sarama.Client)
	this.psubClient = make(map[string]sarama.Client)
	this.zkzone.ForSortedClusters(func(zkcluster *zk.ZkCluster) {
		kfk, err := sarama.NewClient(zkcluster.BrokerList(), sarama.NewConfig())
		swallow(err)

		this.kfkClients[zkcluster.Name()] = kfk
		if this.cluster == zkcluster.Name() {
			this.psubClient[zkcluster.Name()] = kfk
		}

		topics, err := kfk.Topics()
		swallow(err)

		for _, t := range topics {
			this.kafkaTopics[t] = zkcluster.Name()
		}
	})

	mysqlDsns := map[string]string{
		"prod": "user_pubsub:p0nI7mEL6OLW@tcp(m3342.wdds.mysqldb.com:3342)/pubsub?charset=utf8&timeout=10s",
		"sit":  "pubsub:pubsub@tcp(10.209.44.12:10043)/pubsub?charset=utf8&timeout=10s",
		"test": "pubsub:pubsub@tcp(10.209.44.14:10044)/pubsub?charset=utf8&timeout=10s",
	}

	this.mysqlDsn = mysqlDsns[this.zone]

	switch this.mode {
	case "p":
		this.verifyPub()

	case "s":
		this.verifySub()

	case "t":
		this.showTable()
	}

	return
}

func (this *Verify) showTable() {
	this.loadFromManager()

	table := tablewriter.NewWriter(os.Stdout)
	for _, t := range this.topics {
		table.Append([]string{t.AppId, t.TopicName, t.TopicIntro, t.KafkaTopicName})
	}
	table.SetHeader([]string{"Id", "Topic", "Desc", "Kafka"})
	table.Render()
}

func (this *Verify) verifyPub() {
	this.loadFromManager()

	for {
		refreshScreen()

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Kafka", "Stock", "PubSub", "Stock", "Diff"})
		for _, t := range this.topics {
			if t.KafkaTopicName == "" {
				continue
			}
			kafkaCluster := this.kafkaTopics[t.KafkaTopicName]
			if kafkaCluster == "" {
				this.Ui.Warn(fmt.Sprintf("invalid kafka topic: %s", t.KafkaTopicName))
				continue
			}

			psubTopic := manager.KafkaTopic(t.AppId, t.TopicName, "v1")
			offsets := this.pubOffsetDiff(t.KafkaTopicName, kafkaCluster,
				psubTopic, this.cluster)

			table.Append([]string{
				t.KafkaTopicName, fmt.Sprintf("%d", offsets[0]),
				t.TopicName, fmt.Sprintf("%d", offsets[1]),
				color.Red("%d", offsets[0]-offsets[1])})
		}
		table.Render()

		time.Sleep(this.interval)
	}
}

func (this *Verify) pubOffsetDiff(kafkaTopic, kafkaCluster, psubTopic, psubCluster string) []int64 {
	kfk := this.kfkClients[kafkaCluster]
	psub := this.psubClient[psubCluster]

	kp, err := kfk.Partitions(kafkaTopic)
	swallow(err)
	kN := int64(0)
	for _, p := range kp {
		hi, err := kfk.GetOffset(kafkaTopic, p, sarama.OffsetNewest)
		swallow(err)

		lo, err := kfk.GetOffset(kafkaTopic, p, sarama.OffsetOldest)
		swallow(err)

		kN += (hi - lo)
	}

	psp, err := psub.Partitions(psubTopic)
	swallow(err)
	pN := int64(0)
	for _, p := range psp {
		hi, err := psub.GetOffset(psubTopic, p, sarama.OffsetNewest)
		swallow(err)

		lo, err := psub.GetOffset(psubTopic, p, sarama.OffsetOldest)
		swallow(err)

		pN += (hi - lo)
	}

	return []int64{kN, pN}
}

func (this *Verify) verifySub() {

}

func (this *Verify) loadFromManager() {
	db, err := dbx.Open("mysql", this.mysqlDsn)
	swallow(err)

	// TODO fetch from topics_version
	q := db.NewQuery("SELECT AppId, TopicName, TopicIntro, KafkaTopicName FROM topics WHERE Status = 1 ORDER BY CategoryId, TopicName")
	swallow(q.All(&this.topics))
}

func (*Verify) Synopsis() string {
	return "Verify pubsub clients synced with lagacy kafka"
}

func (this *Verify) Help() string {
	help := fmt.Sprintf(`
Usage: %s verify [options]

    Verify pubsub clients synced with lagacy kafka

Options:

    -z zone
      Default %s

    -c cluster

    -mode <p|s|t>

    -i interval
      e,g. 10s


`, this.Cmd, ctx.ZkDefaultZone())
	return strings.TrimSpace(help)
}
