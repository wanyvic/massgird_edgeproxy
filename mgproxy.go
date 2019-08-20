package main

import (
	"context"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	containertypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

var logger *log.Logger
var (
	DefaultDockerAPIVersion  = "1.38"
	DefaultMiningImage       = "massgrid/10.0-autominer-ubuntu16.04:latest"
	ImageKeyFlag             = "imagetype"
	MinerContainerDefaultEnv = "NVIDIA_VISIBLE_DEVICES=all"
	DefaultLeisureMinerName  = "leisureMiner"
	LeisureMinerContainerID  string
	EdgeWorkContainerID      string
	IsMining                 = false
	EdgeWorkRunning          = false
	MinerConfig              MinerOption
)

type MinerOption struct {
	MinerType    string `ETH`
	MinerAddress string `wany`
	MinerWoker   string `worker`
	MinerPool1   string `eth.f2pool.com:6688`
	MinerPool2   string `eth.f2pool.com:8008`
}

func getPathOption(cli *client.Client) error { //get docker info
	info, err := cli.Info(context.Background())
	if err != nil {
		return err
	}
	for _, label := range info.Labels {
		log.Println(label)
		switch {
		case strings.Contains(label, "MINER_TYPE"):
			MinerConfig.MinerType = label[strings.Index(label, "=")+1:]
		case strings.Contains(label, "MINER_ADDRESS"):
			MinerConfig.MinerAddress = label[strings.Index(label, "=")+1:]
		case strings.Contains(label, "MINER_WORKER"):
			MinerConfig.MinerWoker = label[strings.Index(label, "=")+1:]
		case strings.Contains(label, "MINER_POOL1"):
			MinerConfig.MinerPool1 = label[strings.Index(label, "=")+1:]
		case strings.Contains(label, "MINER_POOL2"):
			MinerConfig.MinerPool2 = label[strings.Index(label, "=")+1:]
		}

	}
	log.Println("MinerConfig: ", MinerConfig)
	return nil
}
func getMinerEnv(MinerConfig MinerOption, DefaultEnv string) []string {
	var MinerEnv []string
	MinerEnv = append(MinerEnv, DefaultEnv)
	MinerEnv = append(MinerEnv, "MINER_TYPE="+MinerConfig.MinerType)
	MinerEnv = append(MinerEnv, "MINER_ADDRESS="+MinerConfig.MinerAddress)
	MinerEnv = append(MinerEnv, "MINER_WORKER="+MinerConfig.MinerWoker)
	MinerEnv = append(MinerEnv, "MINER_POOL="+MinerConfig.MinerPool1)
	MinerEnv = append(MinerEnv, "MINER_POOL1="+MinerConfig.MinerPool2)
	return MinerEnv
}
func eventLoop(cli *client.Client) error {

	msg, errors := cli.Events(context.Background(), types.EventsOptions{})
	for {
		// validNameFilter := filters.NewArgs()
		// //validNameFilter.Add("state", "test_name")

		select {
		case m := <-msg:
			log.Println(m)
			if m.Type != "container" {
				continue
			}
			switch m.Action {
			case "start":
				if m.Actor.Attributes["com.massgrid.type"] == "worker" {
					//stop miner
					log.Println("edge work is comming,stop mining")
					go stopMiner(cli)
				}
			case "destroy":
				if m.Actor.Attributes["com.massgrid.type"] == "worker" {
					//start miner
					log.Println("no edge work or miner,starting mining")
					go createMiner(cli, MinerConfig, DefaultMiningImage, DefaultLeisureMinerName)
				}
			case "kill":
				if m.Actor.Attributes["com.massgrid.type"] == "proxy" {

					log.Println("proxy killing,stop mining")
					//stop miner
					go stopMiner(cli)
				}
			}
		case err := <-errors:
			log.Println(err)
		case <-time.After(time.Second * 3):
			log.Println("timeout hit,no msg")
		}
	}
}
func updateStatus(cli *client.Client) error {
	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		return err
	}
	LeisureMinerContainerID = LeisureMinerContainerID[0:0]
	EdgeWorkContainerID = EdgeWorkContainerID[0:0]
	IsMining = false
	EdgeWorkRunning = false
	for _, container := range containers {
		if containerID, exist := isLeisureMinerContainer(container); exist {
			LeisureMinerContainerID = containerID
			IsMining = true
		}
		if containerID, exist := isEdgeWorkCotainer(container); exist {
			EdgeWorkContainerID = containerID
			EdgeWorkRunning = true
		}
	}

	log.Println("updateStatus:", "\n\tMining: ", IsMining, "\n\tcontainerID: ", LeisureMinerContainerID, "\n\tedgeWork: ", EdgeWorkRunning, "\n\tcontainerID: ", EdgeWorkContainerID)
	return nil
}
func initialize(cli *client.Client) error {
	log.Println("initialize")
	getPathOption(cli) //get engine info
	updateStatus(cli)
	if IsMining {
		log.Println("miner has been started,skip initialize")
		return nil
	}
	if EdgeWorkRunning {
		log.Println("edge work has been started,skip initialize")
		return nil
	}

	log.Println("no jobing or mining,miner is starting")
	go createMiner(cli, MinerConfig, DefaultMiningImage, DefaultLeisureMinerName)
	return nil
}
func pullMinerImage(cli *client.Client, strImage string) error {
	log.Println("pulling image\n\tname :", strImage)
	out, err := cli.ImagePull(context.Background(), strImage, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	io.Copy(os.Stdout, out)
	return nil
}
func createMiner(cli *client.Client, option MinerOption, strImage string, containerName string) error {
	log.Println("creating miner:\n\tName:", containerName, "\n\tConfig: ", option, "\n\tImage:", strImage)
	if err := pullMinerImage(cli, strImage); err != nil {
		return err
	}
	minerEnv := getMinerEnv(option, MinerContainerDefaultEnv)
	log.Println(minerEnv)
	containerConfig := containertypes.Config{Env: minerEnv, Image: strImage}
	containerConfig.Labels = make(map[string]string)
	containerConfig.Labels["com.massgrid.type"] = "leisureminer"
	hostConfig := containertypes.HostConfig{CapAdd: []string{"net_admin"}, AutoRemove: true}
	for {
		updateStatus(cli)
		if !IsMining {
			break
		}
	}
	resp, err := cli.ContainerCreate(context.Background(), &containerConfig, &hostConfig, nil, containerName)
	if err != nil {
		log.Println(err)
		return err
	}
	if err := cli.ContainerStart(context.Background(), resp.ID, types.ContainerStartOptions{}); err != nil {
		log.Println(err)
		return err
	}
	updateStatus(cli)
	return nil
}
func stopMiner(cli *client.Client) error {
	log.Println("stoping miner:\n\tID: ", LeisureMinerContainerID)
	if err := cli.ContainerStop(context.Background(), LeisureMinerContainerID, nil); err != nil {
		return err
	}
	updateStatus(cli)
	return nil
}
func isEdgeWorkCotainer(container types.Container) (string, bool) {
	for key, value := range container.Labels {
		if strings.Contains(key, "com.massgrid.type") && value == "worker" {
			return container.ID, true
		}
	}
	return "", false
}
func isLeisureMinerContainer(container types.Container) (string, bool) {
	for _, name := range container.Names {
		if strings.Contains(name, DefaultLeisureMinerName) {
			return container.ID, true
		}
	}
	return "", false
}
func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	cli, err := client.NewClient(client.DefaultDockerHost, DefaultDockerAPIVersion, nil, map[string]string{"Content-type": "application/x-tar"})
	if err != nil {
		panic(err)
	}
	if err := initialize(cli); err != nil {
		panic(err)
	}
	if err := eventLoop(cli); err != nil {
		panic(err)
	}
}
