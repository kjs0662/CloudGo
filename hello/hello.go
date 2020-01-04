package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

//Response return type struct
type Response struct {
	Images []string
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	//GET displays the upload form.
	case "GET":
		files, err := ioutil.ReadDir("/Users/jinseonkim/go/src/hello/storage/")
		if err != nil {
			panic(err)
		}

		var fileArray []string
		for _, file := range files {
			newfile := "/Users/jinseonkim/go/src/hello/storage/" + file.Name()
			if err != nil {
				panic(err)
			}
			if strings.HasSuffix(newfile, "png") {
				fileArray = append(fileArray, newfile)
			}
		}
		fmt.Print(fileArray)
		response := Response{fileArray}
		js, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)

	//POST takes the uploaded file(s) and saves it to disk.
	case "POST":
		//get the multipart reader for the request.
		reader, err := r.MultipartReader()

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			println("Error occur to read data")
			return
		}

		//copy each part to destination.
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}

			//if part.FileName() is empty, skip this iteration.
			if part.FileName() == "" {
				continue
			}

			dst, err := os.Create("/Users/jinseonkim/go/src/hello/storage/" + part.FileName())
			defer dst.Close()

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				println("Error occur to save data")
				return
			}

			if _, err := io.Copy(dst, part); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				println("Error occur to copy data")
				return
			}
		}
		//display success message.
		println("upload", "Upload successful.")
		fmt.Fprint(w, "success")
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func main() {
	http.HandleFunc("/", uploadHandler)

	//static file handler.
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	//Listen on port 9090
	http.ListenAndServe(":9090", nil)
}
