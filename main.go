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
	"sync"
	"sync/atomic"
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

	client := &http.Client{Timeout: 8 * time.Second}
	results := processRecords(records, 200, func(domain string) (string, error) {
		return fetchHash(client, domain)
	})
	cnt := 0
	for _, result := range results {
		if result.err != nil {
			cnt++
		} else if result.row != nil {
			data = append(data, result.row)
		}
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

type hashResult struct {
	row []string
	err error
}

// Each worker owns one result slot at a time; read results only after Wait.
func processRecords(records [][]string, workers int, fetch func(string) (string, error)) []hashResult {
	var completed atomic.Int64
	log.Printf("처리 시작: 입력 %d행, 고루틴 %d개", len(records), workers)
	results := make([]hashResult, len(records))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				func() {
					defer func() {
						if results[i].err != nil {
							log.Println(results[i].err)
						}
						n := completed.Add(1)
						if n%1000 == 0 || n == int64(len(records)) {
							log.Printf("처리 완료: %d/%d행", n, len(records))
						}
					}()
					row := records[i]
					if len(row) == 0 || strings.TrimSpace(row[0]) == "" {
						results[i].err = fmt.Errorf("행 %d: 빈 도메인", i+1)
						return
					}
					domain := strings.TrimSpace(row[0])
					if i == 0 && strings.EqualFold(domain, "domain") {
						return
					}
					hash, err := fetch(domain)
					if err != nil {
						results[i].err = fmt.Errorf("%s: %w", domain, err)
						return
					}
					results[i].row = []string{domain, hash}
				}()
			}
		}()
	}
	for i := range records {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}

func fetchHash(client *http.Client, domain string) (string, error) {
	url := domain
	if !strings.HasPrefix(strings.ToLower(url), "https://") && !strings.HasPrefix(strings.ToLower(url), "http://") {
		url = "https://" + url
	}
	// Use a separate finder per job to avoid sharing library state.
	icons, err := favicon.New().Find(url)
	if err != nil {
		return "", err
	}
	if len(icons) == 0 {
		return "", fmt.Errorf("파비콘 없음")
	}
	fav := icons[0].URL
	for _, icon := range icons {
		if strings.HasSuffix(icon.URL, "/favicon.ico") {
			fav = icon.URL
			break
		}
	}
	resp, err := client.Get(fav)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: HTTP %d", fav, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	hash := int32(murmur3.SeedSum32(0, mmh3FaviconInput(data)))
	return strconv.FormatInt(int64(hash), 10), nil
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
