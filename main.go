package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/schollz/progressbar/v3"
)

type Bar struct {
	bar *progressbar.ProgressBar
	max int64
}

type Split struct {
	Dataset   string `json:"dataset"`
	Config    string `json:"config"`
	SplitName string `json:"split"`
}

type Splits struct {
	Splits []Split `json:"splits"`
}

func main() {
	_ = &Bar{max: 100}

	// get the questions from the api
	fmt.Println("Fetching Splits from the dataset...")
	b, _ := Get("https://datasets-server.huggingface.co/splits?dataset=cais%2Fmmlu")

	var splits Splits
	err := json.Unmarshal([]byte(b), &splits)
	if err != nil {
		panic(err)
	}

	// fetching questions for the fetched splits

	// feeding questions to the hosted model

	// saving the output in output.csv

	//fmt.Println(splits)
}

// init initialises a progress bar
func (b *Bar) init() {
	b.bar = progressbar.Default(b.max)
}

// loadBar loads the progress bar by n units.
func (b *Bar) loadBar(n int) error {
	for i := 0; i < n; i++ {
		err := b.bar.Add(1)
		if err != nil {
			return err
		}
		time.Sleep(40 * time.Millisecond)
	}
	return nil
}

// Get executes the get request over a url and returns the response body as a string
func Get(url string) (string, error) {
	// Create a new HTTP client
	client := &http.Client{}

	// Create a new GET request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	} else if resp.StatusCode != 200 {
		return "", errors.New(resp.Status)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			return
		}
	}(resp.Body)

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Return the response body as a string
	return string(body), nil
}

// getUniqueElements returns a slice of unique integers from the input slice.
func getUniqueElements(arr []Split) []string {
	uniqueMap := make(map[string]struct{}) // Using an empty struct{} to save memory
	var uniqueArr []string

	for _, elem := range arr {

		if _, found := uniqueMap[elem.SplitName]; !found && elem.SplitName != "train" && elem.SplitName != "auxiliary_train" {
			uniqueMap[elem.SplitName] = struct{}{} // Using struct{} instead of bool
			uniqueArr = append(uniqueArr, elem.SplitName)
		}
	}

	return uniqueArr
}
