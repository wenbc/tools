package main

import (
	"errors"
	"github.com/wenbindf/zabbix"
	"io/ioutil"
	"log"
	"os"
	"path"
	"runtime"
	"strings"
	"time"
)

const (
	zabbixApiUrl   = "http://localhost/api_jsonrpc.php"
	zabbixUser     = "admin"
	zabbixPass     = "password"
	gameDir        = "/data/game"             //游戏目录
	gameConfigfile = "configuration.property" //游戏服的配置文件
	gamePortFlag   = "port"                   //游戏配置文件中端口配置 port=4001
	gamePortSeq    = "="                      //配置的分隔符
)

var (
	AllGameNames = make(map[string]string, 0)
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	checkGameDirs(gameDir)
	for {
		select {
		case <-time.After(30 * time.Second):
			checkGameDirs(gameDir)
		}
	}
}

func checkGameDirs(dir string) {
	currentAllGameNames, err := filterGameNames(dir)
	if err != nil {
		log.Fatal("[ERROR] filterGameNames", err)
	}
	currentHostName, err := getLocalHostName()
	if err != nil {
		log.Fatal("[ERROR] getLocalHostName ", err)
	}
	switch {
	case len(currentAllGameNames) < len(AllGameNames):
		//删除zabbix监控项
		for gameName, gamePort := range AllGameNames {
			isDel := true
			for _, currentGameName := range currentAllGameNames {
				//已经添加
				if gameName == currentGameName {
					isDel = false
					break
				}
			}
			if isDel {
				go delZabbixItem(gameName, currentHostName, gamePort)
			}
		}

	case len(currentAllGameNames) > len(AllGameNames):
		//添加zabbix监控项
		for _, gameName := range currentAllGameNames {
			_, ok := AllGameNames[gameName]
			if !ok {
				//未存在监控
				currentGamePort, err := getGamePortString(gameName)
				if err != nil {
					log.Fatal("[ERROR] getGamePortString ", err)
				}
				go addZabbixItem(gameName, currentHostName, currentGamePort)
			}
		}
	}

}
func filterGameNames(dir string) ([]string, error) {
	allGameNames := make([]string, 0)
	FileInfo, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, fi := range FileInfo {
		gameName := fi.Name()
		if fi.IsDir() && strings.HasPrefix(gameName, "game") {
			allGameNames = append(allGameNames, gameName)
		}
	}
	return allGameNames, nil
}

func addZabbixItem(gameName, hostName, gamePort string) {
	zabbixApi := zabbix.NewAPI(zabbixApiUrl)
	zabbixApi.Login(zabbixUser, zabbixPass)
	//添加items和对应的tigger
	log.Println("[INFO] addZabbixItem ", gameName, hostName, gamePort)
	host, err := zabbixApi.HostGetByHost(hostName)
	if err != nil {
		log.Fatal("[ERROR] addZabbixItem.HostGetByHost ", err)
	}
	hostid := host.HostId
	hostname := host.Host
	params := map[string]interface{}{
		"host": hostname,
	}

	items, err := zabbixApi.ItemsGet(params)
	if err != nil {
		log.Fatal("[ERROR] addZabbixItem.ItemsGet ", err)
	}
	interfaceID := ""
	newName := gameName + " game posts " + gamePort
	newKey := "net.tcp.listen[" + gamePort + "]"
	tiggerDescription := "{HOST.NAME} " + gameName + " game posts " + gamePort + " is Down"
	tiggerExpression := "{" + hostName + ":net.tcp.listen[" + gamePort + "].last()}=0"
	//判断key是否已存在
	isExistsItem := IsExistsItems(items, hostid, newKey)
	if isExistsItem {
		log.Println(newName, newKey, "is exists!")
		//zabbix已经存在key,判断对应key是否存在tigger
		var tiggerExits zabbix.TiggerExitsArgs
		tiggerExits.Expression = tiggerExpression
		tiggerExits.Host = hostname
		tiggerExits.HostId = hostid
		isExistsTigger, err := zabbixApi.TiggerExits(tiggerExits)
		if err != nil {
			log.Fatal("[ERROR] addZabbixItem.TiggerExits ", err)
		}
		if !isExistsTigger {
			//添加tigger
			log.Println("tigger is not exists,will add the tigger !", tiggerExpression)
			err := addTigger(zabbixApi, tiggerDescription, tiggerExpression)
			if err != nil {
				log.Fatal("[ERROR] addZabbixItem.TiggerCreate", err)
			}

		}
		addAllGameNamesMap(gameName, gamePort)
		return
	}
	for _, k := range items {
		if k.HostId == hostid {
			if interfaceID == "" {
				interfaceID += k.InterfaceId
			} else {
				break
			}
		}
	}
	var item zabbix.Item
	item.Name = newName
	item.Key = newKey
	item.Type = 0
	item.DataType = 0
	item.HostId = hostid
	item.InterfaceId = interfaceID
	item.Delay = 120
	item.History = 7
	item.Trends = 365
	itemsNew := make([]zabbix.Item, 0)
	itemsNew = append(itemsNew, item)
	log.Println("[INFO] ItemsCreate ", itemsNew)
	err = zabbixApi.ItemsCreate(itemsNew)
	if err != nil {
		log.Fatal("[ERROR] addZabbixItem.ItemsCreate ", err)
	}
	//创建tigger
	err = addTigger(zabbixApi, tiggerDescription, tiggerExpression)
	if err != nil {
		log.Fatal("[ERROR] addZabbixItem.TiggerCreate", err)
	}
	addAllGameNamesMap(gameName, gamePort)
}
func addTigger(zabbixApi *zabbix.API, description, expression string) error {
	var tigger zabbix.Tigger
	tigger.Description = description
	tigger.Expression = expression
	tigger.Priority = 4
	tiggers := make([]zabbix.Tigger, 0)
	tiggers = append(tiggers, tigger)
	err := zabbixApi.TiggerCreate(tiggers)
	if err != nil {
		return err
	}
	log.Println("[INFO] addTigger down!")
	return nil
}
func addAllGameNamesMap(gameName, gamePort string) {
	_, ok := AllGameNames[gameName]
	if !ok {
		AllGameNames[gameName] = gamePort
		log.Println("addAllGameNamesMap down!", gameName)
	}
}
func delAllGameNamesMap(gameName string) {
	_, ok := AllGameNames[gameName]
	if ok {
		delete(AllGameNames, gameName)
		log.Println("delAllGameNamesMap down!", gameName)
	}
}
func getGamePortString(gameName string) (string, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Fatal("[ERROR] getGamePortString", err)
		}
	}()
	gameConfigPath := path.Join(path.Join(gameDir, gameName), gameConfigfile)
	count := 0
	for count < 6 {
		isExistsFile := IsExistsFile(gameConfigPath)
		if isExistsFile {
			break
		}
		time.Sleep(10 * time.Second)
		log.Println("[INFO] getGamePortString ", gameConfigPath, " is not exists!")
		count++
	}

	fd, err := ioutil.ReadFile(gameConfigPath)
	if err != nil {
		return "", err
	}
	log.Println("[INFO] getGamePortString ", gameConfigPath, " is exists!")
	for _, line := range strings.Split(string(fd), "\n") {
		if strings.HasPrefix(line, gamePortFlag) {
			return strings.Split(line, gamePortSeq)[1], nil
		}
	}

	return "", errors.New("配置文件中未找到port字段！" + gameConfigPath)
}
func getLocalHostName() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return hostname, nil
}
func delZabbixItem(gameName, hostName, gamePort string) {
	zabbixApi := zabbix.NewAPI(zabbixApiUrl)
	zabbixApi.Login(zabbixUser, zabbixPass)
	//删除items
	host, err := zabbixApi.HostGetByHost(hostName)
	if err != nil {
		log.Fatal("[ERROR] delZabbixItem.HostGetByHost ", err)
	}
	hostname := host.Host
	params := map[string]interface{}{
		"host": hostname,
	}

	items, err := zabbixApi.ItemsGet(params)
	if err != nil {
		log.Fatal("[ERROR] delZabbixItem.ItemsGet ", err)
	}

	var itemKey zabbix.Item
	for _, k := range items {
		if k.Key == "net.tcp.listen["+gamePort+"]" {
			itemKey = k
		}
	}
	var item zabbix.Item
	item.ItemId = itemKey.ItemId
	item.Delay = itemKey.Delay
	item.HostId = itemKey.HostId
	item.InterfaceId = itemKey.InterfaceId
	item.Key = itemKey.Key
	item.Name = itemKey.Name
	item.Type = itemKey.Type
	item.ValueType = itemKey.ValueType
	item.DataType = itemKey.DataType
	item.Delta = itemKey.Delta
	item.History = itemKey.History
	item.Trends = itemKey.Trends
	itemsNew := make([]zabbix.Item, 0)
	itemsNew = append(itemsNew, item)
	log.Println("[INFO] ItemsDelete ", itemsNew)
	err = zabbixApi.ItemsDelete(itemsNew)
	if err != nil {
		log.Fatal("[ERROR] delZabbixItem.ItemsDelete ", err)
		return
	}
	delAllGameNamesMap(gameName)
}
func IsExistsItems(items []zabbix.Item, hostid, itemKey string) bool {
	for _, k := range items {
		if k.HostId == hostid {
			if k.Key == itemKey {
				return true
			}
		}
	}
	return false
}
func IsExistsFile(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	} else {
		return true
	}
}
