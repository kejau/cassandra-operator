// Copyright 2016 The etcd-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cassandrautil

import (
	"crypto/tls"
	"fmt"

	"github.com/gocql/gocql"
	"github.com/golang/glog"
	"github.com/swarvanusg/go_jolokia"
)

//TODO: Change this to query Cassandra peer tables

type CassandraMember struct {
	Peer           string
	DataCenter     string
	HostId         string
	Rack           string
	ReleaseVersion string
	RPCAddress     string
}

func GetIDbyHost(url string, ip string) (string, error) {
	client := go_jolokia.NewJolokiaClient("http://" + url + ":8778/jolokia/")
	resp, err := client.GetAttr("org.apache.cassandra.db", []string{"type=StorageService"}, "HostIdMap")
	if err != nil {
		glog.Warning("Could not get node membership")
	}
	if membernodes, ok := resp.(map[string]interface{}); ok {
		if data, ok := membernodes[ip]; ok {
			if d, ok := data.(string); ok {
				return d, nil
			} else {
				return "", nil
			}
		} else {
			return "", fmt.Errorf("could not find ID by ip")
		}
	} else {
		return "", fmt.Errorf("could not connect to %s: %v", url, err)
	}

}

func GetMemberNodes(url string) ([]string, error) {
	client := go_jolokia.NewJolokiaClient("http://" + url + ":8778/jolokia/")
	resp, err := client.GetAttr("org.apache.cassandra.db", []string{"type=StorageService"}, "HostIdMap")
	if err != nil {
		glog.Warning("Could not get node membership")
	}

	if membernodes, ok := resp.(map[string]interface{}); ok {
		hosts := make([]string, 0)
		for key, _ := range membernodes {
			hosts = append(hosts, key)
		}
		return hosts, nil
	} else {
		return nil, fmt.Errorf("could not get live nodes list from %s: %v", url, err)
	}
}

func GetDownNodesCount(url string) (int, error) {
	client := go_jolokia.NewJolokiaClient("http://" + url + ":8778/jolokia/")

	resp, err := client.GetAttr("org.apache.cassandra.db", []string{"type=StorageService"}, "UnreachableNodes")
	if err != nil {
		glog.Warning("Could not get UnreachableNodes ")
	}

	if joiningnodes, ok := resp.([]interface{}); ok {
		return len(joiningnodes), nil
	} else {
		return 0, fmt.Errorf("could not get leaving nodes list from %s: %v", url, err)
	}
}


func GetLeavingNodes(url string) ([]string, error) {
	client := go_jolokia.NewJolokiaClient("http://" + url + ":8778/jolokia/")

	resp, err := client.GetAttr("org.apache.cassandra.db", []string{"type=StorageService"}, "LeavingNodes")
	if err != nil {
		glog.Warning("Could not get leaving nodes")
	}

	if joiningnodes, ok := resp.([]string); ok {
		return joiningnodes, nil
	} else {
		return nil, fmt.Errorf("could not get leaving nodes list from %s: %v", url, err)
	}
}

func GetJoiningNodes(url string) ([]string, error) {
	client := go_jolokia.NewJolokiaClient("http://" + url + ":8778/jolokia/")

	resp, err := client.GetAttr("org.apache.cassandra.db", []string{"type=StorageService"}, "JoiningNodes")
	if err != nil {
		glog.Warning("Could not get joining nodes")
	}

	if joiningnodes, ok := resp.([]string); ok {
		return joiningnodes, nil
	} else {
		return nil, fmt.Errorf("could not get joining nodes list from %s: %v", url, err)
	}
}

func ListMembers(clientURLs string, tc *tls.Config) ([]*CassandraMember, error) {
	cluster := gocql.NewCluster(clientURLs)
	cluster.HostFilter = gocql.WhiteListHostFilter(clientURLs)
	cluster.ProtoVersion = 4
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("list members failed: creating cassandra driver failed: %v", err)
	}

	var peer string
	var dataCenter string
	var hostId gocql.UUID
	var preferredIp string
	var rack string
	var releaseVersion string
	var rpcAddress string

	query := session.Query("SELECT peer, data_center, host_id, preferred_ip, rack, release_version, rpc_address FROM system.peers")
	qIter := query.Iter()
	var members = []*CassandraMember{}

	for qIter.Scan(&peer, &dataCenter, &hostId, &preferredIp, &rack, &releaseVersion, &rpcAddress) {
		members = append(members, &CassandraMember{
			Peer:           peer,
			DataCenter:     dataCenter,
			HostId:         hostId.String(),
			Rack:           rack,
			ReleaseVersion: releaseVersion,
			RPCAddress:     rpcAddress,
		})
	}

	localquery := session.Query("SELECT broadcast_address, data_center, host_id, rack, release_version, rpc_address FROM system.local")
	lIter := localquery.Iter()

	for lIter.Scan(&peer, &dataCenter, &hostId, &preferredIp, &rack, &releaseVersion, &rpcAddress) {
		members = append(members, &CassandraMember{
			Peer:           peer,
			DataCenter:     dataCenter,
			HostId:         hostId.String(),
			Rack:           rack,
			ReleaseVersion: releaseVersion,
			RPCAddress:     rpcAddress,
		})
	}

	session.Close()
	return members, err
}

func DecommissionMember(id string) error {
	client := go_jolokia.NewJolokiaClient("http://" + id + ":8778/jolokia/")

	_, err := client.ExecuteOperation("org.apache.cassandra.db:type=StorageService", "decommission", []interface{}{}, "")
	if err != nil {
		glog.Warning("Could not decommission node")
	}
	return err
}


func RemoveNode(id string, target string) error {
	hostId , _ := GetIDbyHost(id, target)
	client := go_jolokia.NewJolokiaClient("http://" + id + ":8778/jolokia/")

	resp, err := client.ExecuteOperation("org.apache.cassandra.db:type=StorageService", "removeNode", []interface{}{hostId}, "")

	glog.Info(resp)
	if err != nil {
		glog.Warning("Could not decommission node")
	}
	return err
}

func CheckHealth(url string, tc *tls.Config) (bool, error) {
	client := go_jolokia.NewJolokiaClient("http://" + url + ":8778/jolokia/")

	resp, err := client.GetAttr("org.apache.cassandra.db", []string{"type=StorageService"}, "LiveNodes")
	if err != nil {
		glog.Warning("Could not reach node, might not be ready")
	}

	if livenodes, ok := resp.([]string); ok {
		for _, b := range livenodes {
			if b == url {
				return true, nil
			}
		}
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("cassandra health probing failed for %s: %v", url, err)
	}
	return true, nil
}
