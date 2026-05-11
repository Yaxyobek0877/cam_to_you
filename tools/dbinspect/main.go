// DB tekshirish toolchasi.
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	appData := os.Getenv("APPDATA")
	dbPath := filepath.Join(appData, "Cam2You", "app.db")
	fmt.Printf("DB: %s\n", dbPath)

	info, err := os.Stat(dbPath)
	if err != nil {
		fmt.Printf("DB faylga kira olmadik: %v\n", err)
		return
	}
	fmt.Printf("Hajm: %d bayt\n\n", info.Size())

	conn, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		fmt.Printf("Ochib bo'lmadi: %v\n", err)
		return
	}
	defer conn.Close()

	// Versiya
	var version int
	conn.QueryRow("SELECT MAX(version) FROM _schema_version").Scan(&version)
	fmt.Printf("Schema versiyasi: v%d\n\n", version)

	fmt.Println("=== KAMERALAR ===")
	camRows, err := conn.Query(`SELECT id, name, vendor, host, port, username, channel, use_sub_stream FROM cameras`)
	if err != nil {
		fmt.Printf("Cameras o'qib bo'lmadi: %v\n", err)
		return
	}
	count := 0
	for camRows.Next() {
		var id int64
		var name, vendor, host, username string
		var port, channel, useSub int
		camRows.Scan(&id, &name, &vendor, &host, &port, &username, &channel, &useSub)
		fmt.Printf("#%d  %s  [%s]  %s@%s:%d  ch=%d  sub=%v\n",
			id, name, vendor, username, host, port, channel, useSub == 1)
		count++
	}
	camRows.Close()
	fmt.Printf("Jami: %d kamera\n\n", count)

	fmt.Println("=== STREAMLAR ===")
	strRows, err := conn.Query(`SELECT id, name, layout, camera_ids, quality, encoder, platform, stream_key, custom_url, auto_restart, max_restarts, restart_delay_ms FROM streams`)
	if err != nil {
		fmt.Printf("Streamlar o'qib bo'lmadi: %v\n", err)
		return
	}
	count = 0
	for strRows.Next() {
		var id int64
		var name, layout, camIDs, quality, encoder, platform, streamKey, customURL string
		var autoRestart, maxRestarts, restartDelay int
		strRows.Scan(&id, &name, &layout, &camIDs, &quality, &encoder, &platform, &streamKey, &customURL, &autoRestart, &maxRestarts, &restartDelay)

		keyDisplay := streamKey
		if len(keyDisplay) > 8 {
			keyDisplay = streamKey[:4] + "..." + streamKey[len(streamKey)-4:]
		}
		if streamKey == "" {
			keyDisplay = "(BO'SH!)"
		}

		fmt.Printf("#%d  %s\n", id, name)
		fmt.Printf("    Layout:      %s\n", layout)
		fmt.Printf("    CameraIDs:   %s\n", camIDs)
		fmt.Printf("    Quality:     %s\n", quality)
		fmt.Printf("    Encoder:     %s\n", encoder)
		fmt.Printf("    Platform:    %s\n", platform)
		fmt.Printf("    Stream key:  %s (%d belgi)\n", keyDisplay, len(streamKey))
		if customURL != "" {
			fmt.Printf("    Custom URL:  %s\n", customURL)
		}
		fmt.Printf("    AutoRestart: %v (max %d, delay %dms)\n", autoRestart == 1, maxRestarts, restartDelay)
		fmt.Println()
		count++
	}
	strRows.Close()
	fmt.Printf("Jami: %d stream\n", count)
}
