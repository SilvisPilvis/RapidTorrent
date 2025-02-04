package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
)

func printProgress(progress float64) {
	width := 50
	filled := int(progress * float64(width))
	empty := width - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	fmt.Printf("\r[%s] %.1f%%", bar, progress*100)
}

func main() {
	c, _ := torrent.NewClient(nil)
	defer c.Close()

	t, _ := c.AddMagnet("magnet:?xt=urn:btih:345fdfabef018a7946cd0a0d046fb714e6f39683&dn=%5BJudas%5D%20Loop%207-kaime%20no%20Akuyaku%20Reijou%20wa%2C%20Moto%20Tekikoku%20de%20Jiyuu%20Kimamana%20Hanayome%20Seikatsu%20wo%20Mankitsu%20Suru%20%28Season%2001%29%20%5B1080p%5D%5BHEVC%20x265%2010bit%5D%5BMulti-Subs%5D%20%28Batch%29&tr=http%3A%2F%2Fnyaa.tracker.wf%3A7777%2Fannounce&tr=udp%3A%2F%2Fopen.stealth.si%3A80%2Fannounce&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337%2Fannounce&tr=udp%3A%2F%2Fexodus.desync.com%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.torrent.eu.org%3A451%2Fannounce")

	<-t.GotInfo()
	t.DownloadAll()

	log.Print("torrent started")
	for _, item := range c.Torrents() {
		log.Printf("Downloading %s", item.Name())
		log.Printf("Downloading %s", item.InfoHash().String())
	}

	// Progress monitoring loop
	for {
		stats := t.Stats()
		total := float64(t.Length())
		completed := float64(stats.BytesRead.Int64())

		if total > 0 {
			progress := completed / total
			printProgress(progress)
		}

		// Check if download is complete
		if t.Complete().Bool() {
			fmt.Println("\nDownload completed!")
			break
		}

		time.Sleep(time.Millisecond * 500)
	}

	c.WaitAll()
	log.Print("torrent downloaded")
}
