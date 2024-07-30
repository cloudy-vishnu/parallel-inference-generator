package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
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

type Questions struct {
	TotalRows int   `json:"num_rows_total"`
	Rows      []Row `json:"rows"`
}
type Row struct {
	Question QuestionDetail `json:"row"`
}

type QuestionDetail struct {
	Question      string   `json:"question"`
	CorrectAnswer int      `json:"answer"`
	Choices       []string `json:"choices"`
}

type Metrics struct {
	ComputeTime    string `json:"compute_time"`
	TotalTime      string `json:"total_time"`
	InferenceTime  string `json:"inference_time"`
	TokenRate      string `json:"time_per_token"`
	PromptToken    string `json:"prompt_token"`
	GeneratedToken string `json:"generated_token"`
}

type GeneratedResponse struct {
	GeneratedText string  `json:"generated_text"`
	Metrics       Metrics `json:"metrics"`
}

type RequestBody struct {
	InputPrompt string                 `json:"inputs"`
	Parameters  map[string]interface{} `json:"parameters"`
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

// main is the entry point to the program
func main() {
	// get the splits from the api
	fmt.Println("Fetching Splits from the dataset...")
	b, _ := Get("https://datasets-server.huggingface.co/splits?dataset=cais%2Fmmlu")

	var splits Splits
	err := json.Unmarshal([]byte(b), &splits)
	if err != nil {
		panic(err)
	}
	splitsArray := getUniqueElements(splits.Splits)

	// fetching questions for the fetched splits
	fmt.Println("Fetching Questions from the splits...")
	var questions Questions
	wg := &sync.WaitGroup{}
	for _, split := range splitsArray {
		wg.Add(1)
		go func(split string) {
			q, err := GetQuestionsFromSplit(split)
			if err != nil {
				panic(err)
			}
			questions.Rows = append(questions.Rows, q.Rows...)
			questions.TotalRows += len(q.Rows)
			fmt.Println("FOR SPLIT:", split, ", QUESTIONS RECEIVED:", len(q.Rows))
			wg.Done()
		}(split)
	}
	wg.Wait()

	// feeding questions to the hosted model
	fmt.Println("Feeding Questions", questions.TotalRows, "to the hosted model...")
	wg = &sync.WaitGroup{}
	for i := 0; i < len(questions.Rows); i += 255 {
		for j := 0; j < 256; j++ {
			rb := RequestBody{
				InputPrompt: FormatQuestion(questions.Rows[i+j].Question),
				Parameters: map[string]interface{}{
					"stop": []string{"<|start_header_id|>", "<|end_header_id|>", "<|eot_id|>", "<|reserved_special_token"},
				},
			}
			jsonBody, err := json.Marshal(rb)
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				response, err := Post("http://54.224.158.96:8080/generate", string(jsonBody))
				if err != nil {
					panic(err)
				}
				fmt.Println("Response Received", response)
				wg.Done()
			}()
		}
	}
	wg.Wait()

	// saving the output in output.csv
	fmt.Println("Output saved to output.csv!")
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

func GetQuestionsFromSplit(split string) (Questions, error) {
	var offset int

	body, err := Get("https://datasets-server.huggingface.co/rows?dataset=cais%2Fmmlu&config=all&split=" + split + "&offset=" + fmt.Sprintf("%d", offset))
	if err != nil {
		return Questions{}, err
	}

	var q Questions
	err = json.Unmarshal([]byte(body), &q)
	if err != nil {
		return Questions{}, err
	}
	wg := &sync.WaitGroup{}
	for i := 100; i < q.TotalRows; i += 100 {
		wg.Add(1)
		go func() {
			err := func() error {
				defer wg.Done()
				body2, err := Get("https://datasets-server.huggingface.co/rows?dataset=cais%2Fmmlu&config=all&split=" + split + "&offset=" + fmt.Sprintf("%d", i))
				if err != nil {

					return err
				}
				var q2 Questions
				err = json.Unmarshal([]byte(body2), &q2)
				if err != nil {
					return err
				}

				q.Rows = append(q.Rows, q2.Rows...)
				return nil
			}()
			if err != nil {
				panic(err)
			}
		}()
	}
	wg.Wait()
	return q, nil
}

func Post(url, body string) (GeneratedResponse, error) {
	// Create a new HTTP client
	client := &http.Client{}

	// Create a new GET request
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return GeneratedResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		return GeneratedResponse{}, err
	} else if resp.StatusCode != 200 {
		return GeneratedResponse{}, errors.New(resp.Status)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			return
		}
	}(resp.Body)

	// Read the response body
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return GeneratedResponse{}, err
	}
	fmt.Println("RESPONSE BODY STRING: ", b)
	var g GeneratedResponse
	err = json.Unmarshal(b, &g)
	if err != nil {
		return GeneratedResponse{}, err
	}

	g.Metrics.ComputeTime = resp.Header.Get("x-compute-time")
	g.Metrics.TotalTime = resp.Header.Get("x-total-time")
	g.Metrics.InferenceTime = resp.Header.Get("x-inference-time")
	g.Metrics.TokenRate = resp.Header.Get("x-time-per-token")
	g.Metrics.PromptToken = resp.Header.Get("x-prompt-tokens")
	g.Metrics.GeneratedToken = resp.Header.Get("x-generated-tokens")

	// Return the response body as a string
	return g, nil
}

func FormatQuestion(detail QuestionDetail) string {
	var q string
	for _, choice := range detail.Choices {
		q += choice
	}
	return q
}
