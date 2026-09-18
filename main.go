package main

import (
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/thanhpk/go-favicon"
	"github.com/twmb/murmur3"
)

func main() {
	file, err := os.Open("favicon.csv")
	if err != nil {
		log.Fatal(err)
	}
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	file.Close()
	if err != nil {
		log.Fatal(err)
	}
	data := [][]string{
		{"domain", "favicon_hash"},
	}

	client := &http.Client{Timeout: 15 * time.Second}
	cnt := 0
	for i, row := range records {
		if len(row) == 0 || strings.TrimSpace(row[0]) == "" {
			cnt++
			continue
		}
		domain := strings.TrimSpace(row[0])
		if i == 0 && strings.EqualFold(domain, "domain") {
			continue
		}
		url := domain
		if !strings.HasPrefix(strings.ToLower(url), "https://") && !strings.HasPrefix(strings.ToLower(url), "http://") {
			url = "https://" + url
		}
		icon, err := favicon.Find(url)
		if err != nil {
			log.Println(err)
			cnt++
			continue
		}
		if len(icon) == 0 {
			cnt++
			log.Println("파비콘 없음")
			continue
		}
		var fav string
		for _, ic := range icon {
			if strings.HasSuffix(ic.URL, "/favicon.ico") {
				fav = ic.URL
				break
			}
		}
		if fav == "" {
			fav = icon[0].URL
		}
		resp, err := client.Get(fav)
		if err != nil {
			log.Println(err)
			cnt++
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			log.Printf("%s: HTTP %d", fav, resp.StatusCode)
			cnt++
			continue
		}
		favImgBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Println(err)
			cnt++
			continue
		}
		hash := int32(murmur3.SeedSum32(0, mmh3FaviconInput(favImgBytes)))
		data = append(data, []string{domain, strconv.FormatInt(int64(hash), 10)})
	}

	file, err = os.Create("favicon_list.csv")
	if err != nil {
		log.Fatal(err)
	}
	writer := csv.NewWriter(file)
	err = writer.WriteAll(data)
	if err != nil {
		log.Fatal(err)
	}
	if err := file.Close(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("실패 개수 : %d\n", cnt)
}

func mmh3FaviconInput(data []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(data)
	var result strings.Builder

	for len(encoded) > 0 {
		n := min(76, len(encoded))
		result.WriteString(encoded[:n])
		result.WriteByte('\n')
		encoded = encoded[n:]
	}

	return []byte(result.String())
}
