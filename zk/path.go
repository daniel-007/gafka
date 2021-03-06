package zk

import (
	"fmt"
)

const (
	clusterRoot     = "/_kafka_clusters"
	clusterInfoRoot = "/_kafa_clusters_info"

	KatewayIdsRoot     = "/_kateway/ids"
	katewayMetricsRoot = "/_kateway/metrics"
	KatewayMysqlPath   = "/_kateway/mysql"

	PubsubJobConfig      = "/_kateway/orchestrator/jobconfig"
	PubsubJobQueues      = "/_kateway/orchestrator/jobs"
	PubsubActors         = "/_kateway/orchestrator/actors/ids"
	PubsubJobQueueOwners = "/_kateway/orchestrator/actors/job_owners"
	PubsubWebhooks       = "/_kateway/orchestrator/webhooks"
	PubsubWebhooksOff    = "/_kateway/orchestrator/webhooks_off"
	PubsubWebhookOwners  = "/_kateway/orchestrator/actors/webhook_owners"
	//PubsubActorRebalance = "/_kateway/orchestrator/rebalance"

	KguardLeaderPath = "_kguard/leader"

	ConsumersPath           = "/consumers"
	BrokerIdsPath           = "/brokers/ids"
	BrokerTopicsPath        = "/brokers/topics"
	ControllerPath          = "/controller"
	ControllerEpochPath     = "/controller_epoch"
	BrokerSequenceIdPath    = "/brokers/seqid"
	EntityConfigChangesPath = "/config/changes"
	TopicConfigPath         = "/config/topics"
	EntityConfigPath        = "/config"
	DeleteTopicsPath        = "/admin/delete_topics"

	RedisMonPath     = "/redis"
	RedisClusterRoot = "/rediscluster"

	DbusRoot           = "/dbus"
	dbusConfPath       = "conf"
	dbusCheckpointPath = "checkpoint"
	dbusConfDirPath    = "conf.d"
	dbusClusterPath    = "cluster"

	// ElasticSearch
	esRoot = "/_es"
)

func katewayMetricsRootByKey(id, key string) string {
	return fmt.Sprintf("%s/%s/%s", katewayMetricsRoot, id, key)
}

func esClusterPath(cluster string) string {
	return fmt.Sprintf("%s/%s", esRoot, cluster)
}

func ClusterPath(cluster string) string {
	return fmt.Sprintf("%s/%s", clusterRoot, cluster)
}

func (this *ZkCluster) controllerPath() string {
	return this.path + ControllerPath
}

func (this *ZkCluster) TopicConfigRoot() string {
	return fmt.Sprintf("%s%s", this.path, TopicConfigPath)
}

func (this *ZkCluster) GetTopicConfigPath(topic string) string {
	return fmt.Sprintf("%s%s/%s", this.path, TopicConfigPath, topic)
}

func (this *ZkCluster) ClusterInfoPath() string {
	return fmt.Sprintf("%s/%s", clusterInfoRoot, this.name)
}

func (this *ZkCluster) controllerEpochPath() string {
	return this.path + ControllerEpochPath
}

func (this *ZkCluster) partitionsPath(topic string) string {
	return fmt.Sprintf("%s%s/%s/partitions", this.path, BrokerTopicsPath, topic)
}

func (this *ZkCluster) partitionStatePath(topic string, partitionId int32) string {
	return fmt.Sprintf("%s/%d/state", this.partitionsPath(topic), partitionId)
}

func (this *ZkCluster) topicsRoot() string {
	return this.path + BrokerTopicsPath
}

func (this *ZkCluster) brokerIdsRoot() string {
	return this.path + BrokerIdsPath
}

func (this *ZkCluster) brokerPath(id int) string {
	return fmt.Sprintf("%s/%d", this.brokerIdsRoot(), id)
}

func (this *ZkCluster) consumerGroupsRoot() string {
	return this.path + ConsumersPath
}

func (this *ZkCluster) ConsumerGroupRoot(group string) string {
	return this.path + ConsumersPath + "/" + group
}

func (this *ZkCluster) consumerGroupIdsPath(group string) string {
	return this.ConsumerGroupRoot(group) + "/ids"
}

func (this *ZkCluster) ConsumerGroupOffsetPath(group string) string {
	return this.ConsumerGroupRoot(group) + "/offsets"
}

func (this *ZkCluster) consumerGroupOffsetOfTopicPath(group, topic string) string {
	return this.ConsumerGroupOffsetPath(group) + "/" + topic
}

func (this *ZkCluster) consumerGroupOffsetOfTopicPartitionPath(group, topic, partition string) string {
	return this.consumerGroupOffsetOfTopicPath(group, topic) + "/" + partition
}

func (this *ZkCluster) consumerGroupOwnerOfTopicPath(group, topic string) string {
	return this.ConsumerGroupRoot(group) + "/owners/" + topic
}
