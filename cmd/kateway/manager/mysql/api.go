package mysql

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash/adler32"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/funkygao/gafka/cmd/kateway/manager"
	"github.com/funkygao/gafka/cmd/kateway/structs"
	"github.com/funkygao/gafka/mpool"
)

const (
	maxTopicLen = 64
)

var (
	topicNameRegex = regexp.MustCompile(`[a-zA-Z0-9\-_]+`)
)

func (this *mysqlStore) TopicAppid(kafkaTopic string) string {
	firstDot := strings.IndexByte(kafkaTopic, '.')
	if firstDot == -1 || firstDot > len(kafkaTopic) {
		return ""
	}
	return kafkaTopic[:firstDot]
}

func (this *mysqlStore) KafkaTopic(appid string, topic string, ver string) (r string) {
	b := mpool.BytesBufferGet()
	b.Reset()
	b.WriteString(appid)
	b.WriteByte('.')
	b.WriteString(topic)
	b.WriteByte('.')
	b.WriteString(ver)
	if len(ver) > 2 {
		// ver starts with 'v1', from 'v10' on, will use obfuscation
		b.WriteByte('.')

		// can't use app secret as part of cookie: what if user changes his secret?
		// FIXME user can guess the cookie if they know the algorithm in advance
		cookie := adler32.Checksum([]byte(appid + topic))
		b.WriteString(strconv.Itoa(int(cookie % 1000)))
	}
	r = b.String()
	mpool.BytesBufferPut(b)
	return
}

func (this *mysqlStore) Signature(appid string) string {
	if secret, present := this.appSecretMap[appid]; present {
		src := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", appid, secret)))
		return base64.URLEncoding.EncodeToString(src[:])
	} else {
		return ""
	}
}

func (this *mysqlStore) TopicSchema(appid, topic, ver string) (string, error) {
	atv := structs.AppTopicVer{AppID: appid, Topic: topic, Ver: ver}
	if schema, present := this.topicSchemaMap[atv]; present {
		return schema, nil
	}

	return "", manager.ErrSchemaNotFound
}

func (this *mysqlStore) ShadowTopic(shadow, myAppid, hisAppid, topic, ver, group string) (r string) {
	r = this.KafkaTopic(hisAppid, topic, ver)
	return r + "." + myAppid + "." + group + "." + shadow
}

func (this *mysqlStore) Dump() map[string]interface{} {
	r := make(map[string]interface{})
	r["app_cluster"] = this.appClusterMap
	r["subscrptions"] = this.appSubMap
	r["app_topic"] = this.appTopicsMap
	r["groups"] = this.appConsumerGroupMap
	r["shadows"] = this.shadowQueueMap
	r["dryrun"] = this.dryrunTopics
	return r
}

func (this *mysqlStore) DeadPartitions() map[string]map[int32]struct{} {
	return this.deadPartitionMap
}

func (this *mysqlStore) ForceRefresh() {
	this.refreshCh <- struct{}{}
}

func (this *mysqlStore) ValidateTopicName(topic string) bool {
	return len(topic) > 0 && len(topic) <= maxTopicLen && topicNameRegex.FindString(topic) == topic
}

func (this *mysqlStore) ValidateGroupName(header http.Header, group string) bool {
	if len(group) == 0 {
		return false
	}

	for _, c := range group {
		if !(c == '_' || c == '-' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}

	if group == "__smoketest__" && header.Get("X-Origin") != "smoketest" {
		return false
	}

	return true
}

func (this *mysqlStore) AuthAdmin(appid, pubkey string) bool {
	if appid == "_psubAdmin_" && pubkey == "_wandafFan_" { // FIXME
		return true
	}

	return false
}

func (this *mysqlStore) OwnTopic(appid, pubkey, topic string) error {
	if appid == "" || topic == "" || pubkey == "" {
		return manager.ErrEmptyIdentity
	}

	// authentication
	if secret, present := this.appSecretMap[appid]; !present || pubkey != secret {
		return manager.ErrAuthenticationFail
	}

	// authorization
	if enabled, present := this.appTopicsMap[structs.AppTopic{AppID: appid, Topic: topic}]; present {
		if enabled {
			return nil
		} else {
			return manager.ErrDisabledTopic
		}
	}

	return manager.ErrAuthorizationFail
}

func (this *mysqlStore) AllowSubWithUnregisteredGroup(yesOrNo bool) {
	this.allowUnregisteredGroup = yesOrNo
}

func (this *mysqlStore) AuthSub(appid, subkey, hisAppid, hisTopic, group string) error {
	if appid == "" || hisTopic == "" {
		return manager.ErrEmptyIdentity
	}

	// authentication
	if secret, present := this.appSecretMap[appid]; !present || subkey != secret {
		return manager.ErrAuthenticationFail
	}

	// group verification
	if !this.allowUnregisteredGroup {
		if group == "" {
			// empty group, means we skip group verification
		} else if group != "__smoketest__" {
			if _, present := this.appConsumerGroupMap[structs.AppGroup{AppID: appid, Group: group}]; !present {
				// user must register group before Sub
				return manager.ErrInvalidGroup
			}
		}
	}

	if appid == hisAppid {
		// sub my own topic is always authorized FIXME what if the topic is disabled?
		return nil
	}

	// authorization
	if _, present := this.appSubMap[structs.AppTopic{AppID: appid, Topic: hisTopic}]; present {
		return nil
	}

	return manager.ErrAuthorizationFail
}

func (this *mysqlStore) LookupCluster(appid string) (string, bool) {
	if cluster, present := this.appClusterMap[appid]; present {
		return cluster, present
	}

	return "", false
}

func (this *mysqlStore) IsShadowedTopic(hisAppid, topic, ver, myAppid, group string) bool {
	if _, present := this.shadowQueueMap[this.shadowKey(hisAppid, topic, ver, myAppid)]; present {
		return true
	}

	return false
}

func (this *mysqlStore) IsDryrunTopic(appid, topic, ver string) bool {
	this.dryrunLock.RLock()
	_, present := this.dryrunTopics[structs.AppTopicVer{AppID: appid, Topic: topic, Ver: ver}]
	this.dryrunLock.RUnlock()

	return present
}

func (this *mysqlStore) MarkTopicDryrun(appid, topic, ver string) {
	this.dryrunLock.Lock()
	this.dryrunTopics[structs.AppTopicVer{AppID: appid, Topic: topic, Ver: ver}] = struct{}{}
	this.dryrunLock.Unlock()
}

func (this *mysqlStore) ClearDryrunTopics() {
	this.dryrunLock.Lock()
	this.dryrunTopics = make(map[structs.AppTopicVer]struct{})
	this.dryrunLock.Unlock()
}
