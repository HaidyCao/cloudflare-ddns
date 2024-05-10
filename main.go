package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	UrlParseTypeFull = "full"
	UrlParseTypeJson = "json"
)

var (
	apiKey       string
	zoneId       string
	domainType   string
	name         string
	ttl          int
	url          string
	newIp        string
	interval     int
	urlParseType string
	urlJsonPath  string
)

type RecordResult struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

type RecordResponse struct {
	Success bool           `json:"success"`
	Result  []RecordResult `json:"result"`
}

type OverwriteRecordResponse struct {
	Success bool `json:"success"`
}

type PutRecord struct {
	Content string   `json:"content"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Proxied bool     `json:"proxied"`
	Comment string   `json:"comment"`
	Tags    []string `json:"tags"`
	TTL     int      `json:"ttl"`
}

func main() {
	flag.StringVar(&apiKey, "a", "", "cloudflare api key")
	flag.StringVar(&domainType, "t", "A", "domain type")
	flag.StringVar(&name, "n", "@", "domain name")
	flag.IntVar(&ttl, "T", 60, "TTL")
	flag.StringVar(&url, "u", "http://api.ipify.org", "default check url")
	flag.StringVar(&newIp, "N", "", "new ip")
	flag.IntVar(&interval, "i", 5*60, "interval")
	flag.StringVar(&urlParseType, "p", UrlParseTypeFull, "url parse type: full(default) or json")
	flag.StringVar(&urlJsonPath, "j", "", "url json path")
	flag.StringVar(&zoneId, "z", "", "cloudflare zone id")

	flag.Parse()

	if apiKey == "" {
		flag.PrintDefaults()
		return
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second) // 创建一个每隔5分钟触发一次的定时器
	defer ticker.Stop()                                             // 确保在函数返回时停止定时器

	for {
		updateIp()
		select {
		case <-ticker.C: // 当定时器触发时执行以下代码
			fmt.Println("Ticker ticked at", time.Now())
			// 在这里添加你希望每次定时触发时执行的代码
		}
	}
}

func updateIp() {
	var ip string
	if newIp == "" {
		log.Println(domainType, name, ttl, url, zoneId)
		resp, err := http.Get(url)
		if err != nil {
			log.Println(err)
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		ipBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println(err)
			return
		}
		if urlParseType == UrlParseTypeFull {
			ip = string(ipBytes)
		} else {
			var data map[string]interface{}
			err := json.Unmarshal(ipBytes, &data)
			if err != nil {
				fmt.Println("解析 JSON 失败:", string(ipBytes), err)
				return
			}

			value, ok := getNestedValue(data, urlJsonPath)
			if !ok {
				fmt.Println("未找到 ip 地址", string(ipBytes))
				return
			}

			if strValue, ok := value.(string); ok {
				fmt.Println("ip 值为:", strValue)
				ip = strValue
			}
		}
	} else {
		ip = newIp
	}

	log.Printf("current ip = %s", ip)

	dnsRecordsUrl := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", zoneId)
	request, err := http.NewRequest("GET", dnsRecordsUrl, nil)
	if err != nil {
		log.Println(err)
		return
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	remoteResp, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Println(err)
		return
	}
	defer func() {
		_ = remoteResp.Body.Close()
	}()

	remoteBytes, err := io.ReadAll(remoteResp.Body)
	if err != nil {
		log.Println(err)
		return
	}

	var respBody RecordResponse
	err = json.Unmarshal(remoteBytes, &respBody)
	if err != nil {
		log.Printf("respBody = %s, err = %v", remoteBytes, err)
		return
	}

	if !respBody.Success {
		log.Printf("request failed: respBody = %s", remoteBytes)
		return
	}

	//var hasCurrentIp = false
	dnsRecordId := ""
	for _, record := range respBody.Result {
		log.Printf("remote ip = %s", record.Content)

		if record.Content == ip && record.Type == domainType {
			//hasCurrentIp = true
			dnsRecordId = record.Id
			break
		}
	}

	//if hasCurrentIp {
	//	log.Println("No update required")
	//	return
	//}

	putBody := PutRecord{
		Content: ip,
		TTL:     ttl,
		Name:    name,
		Type:    domainType,
		Proxied: false,
	}

	putBodyBytes, err := json.Marshal(putBody)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println(string(putBodyBytes))

	updateUrl := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneId, dnsRecordId)
	updateRequest, err := http.NewRequest("PUT", updateUrl, bytes.NewReader(putBodyBytes))
	if err != nil {
		log.Println(err)
		return
	}

	updateRequest.Header.Set("Content-Type", "application/json")
	updateRequest.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	resultResp, err := http.DefaultClient.Do(updateRequest)
	if err != nil {
		log.Println(err)
		return
	}

	if resultResp.StatusCode == 200 {
		remoteBytes, err = io.ReadAll(resultResp.Body)
		if err != nil {
			log.Println(err)
			return
		}

		var result OverwriteRecordResponse
		err = json.Unmarshal(remoteBytes, &result)
		if err != nil {
			log.Printf("respBody = %s, err = %v", remoteBytes, err)
			return
		}

		if result.Success {
			log.Printf("update success")
		} else {
			log.Printf("update failed: %s", remoteBytes)
		}
		return
	}

	resultBodyBytes, err := io.ReadAll(resultResp.Body)
	if err != nil {
		log.Println(err)
		return
	}

	log.Println("update failed: " + string(resultBodyBytes))
}
