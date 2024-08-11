package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"github.com/fatih/color"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/joho/godotenv"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type GenericData struct {
	Timestamp time.Time
	Value     float64
}

func main() {
	files := map[string]string{
		"raw_tracker_steps.csv":    "steps",
		"raw_hr_hr.csv":            "heart_rate",
		"raw_tracker_distance.csv": "distance",
	}

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	folderPath := flag.String("folder", "", "Folder containing withings export CSV files")
	flag.Parse()

	// Check if folder exist
	if _, err := os.Stat(*folderPath); os.IsNotExist(err) {
		color.Red("Folder %s not found", *folderPath)
	}

	// Loop over each supported files
	for fileName, measurement := range files {
		filePath := filepath.Join(*folderPath, fileName)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			fmt.Printf("File : %s doesn't exist\n", filePath)
			continue
		}
		fmt.Printf("Reading file %s\n", filePath)
		// Open file
		file, err := os.Open(filePath)
		if err != nil {
			panic(err)
		}
		defer file.Close()

		reader := csv.NewReader(file)

		records, err := reader.ReadAll()
		if err != nil {
			panic(err)
		}

		// All steps will be stored here
		var dataPoints []GenericData

		fmt.Printf("Found a total of %d records in CSV file\n", len(records))

		_, err = reader.Read() // skip first line
		if err != nil {
			if err != io.EOF {
				log.Fatalln(err)
			}
		}

		for _, record := range records {
			// Time is in first column
			start, err := time.Parse(time.RFC3339, record[0])
			if err != nil {
				panic(err)
			}

			// We have a slice of int
			durations := parseIntSlice(record[1])
			var data []float64
			if measurement == "distance" {
				data = parseFloatSlice(record[2])
			} else {
				intData := parseIntSlice(record[2])
				data = make([]float64, len(intData))
				for i, v := range intData {
					data[i] = float64(v)
				}
			}

			// Loop over each duration to create an event with time and steps
			for i, duration := range durations {
				dataPoints = append(dataPoints, GenericData{
					Timestamp: start.Add(time.Duration(duration) * time.Second),
					Value:     data[i],
				})
			}
		}

		fmt.Println("Going to write data to InfluxDB")
		client := influxdb2.NewClient(os.Getenv("INFLUXDB_URL"), os.Getenv("TOKEN"))
		defer client.Close()

		writeAPI := client.WriteAPI(os.Getenv("INFLUXDB_ORG"), os.Getenv("INFLUXDB_BUCKET"))

		for _, data := range dataPoints {
			p := influxdb2.NewPoint(
				measurement,
				map[string]string{"device": "scanwatch"},
				map[string]interface{}{"count": data.Value},
				data.Timestamp,
			)
			writeAPI.WritePoint(p)
		}

		writeAPI.Flush()
		color.Green("Data successfully written to InfluxDB")
	}
}

func parseIntSlice(s string) []int {
	s = strings.Trim(s, "[]")
	parts := strings.Split(s, ",")
	result := make([]int, len(parts))
	for i, part := range parts {
		val, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			log.Printf("Error parsing integer: %v", err)
			continue
		}
		result[i] = val
	}
	return result
}

func parseFloatSlice(s string) []float64 {
	s = strings.Trim(s, "[]")
	parts := strings.Split(s, ",")
	result := make([]float64, len(parts))
	for i, part := range parts {
		val, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			log.Printf("Error parsing float: %v", err)
			continue
		}
		result[i] = val
	}
	return result
}
