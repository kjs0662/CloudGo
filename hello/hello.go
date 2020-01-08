package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

//Response return type struct
type Response struct {
	Images []string
}

type Info struct {
	Identifier  string
	CreatedDate string
}

type PhotoModel struct {
	Identifier  string
	Image       string
	CreatedDate string
}

var client *mongo.Client

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	//GET displays the upload form.
	case "GET":
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
		if err != nil {
			panic(err)
		}

		collection := client.Database("photocloud").Collection("photo")
		cursor, err := collection.Find(ctx, bson.M{})
		if err != nil {
			panic(err)
		}
		defer cursor.Close(ctx)
		var photoArray []PhotoModel
		for cursor.Next(ctx) {
			var photo PhotoModel
			cursor.Decode(&photo)
			photoArray = append(photoArray, photo)
		}

		json.NewEncoder(w).Encode(photoArray)

	//POST takes the uploaded file(s) and saves it to disk.
	case "POST":
		//get the multipart reader for the request.
		reader, err := r.MultipartReader()

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			println("Error occur to read data")
			return
		}

		var photoArray []PhotoModel
		index := 0
		//copy each part to destination.
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}

			if part.FileName() == "" {
				//pasing json here

				buf := new(bytes.Buffer)
				buf.ReadFrom(part)
				s := buf.String()

				bytes := []byte(s)
				var infos []Info
				json.Unmarshal(bytes, &infos)
				for l := range infos {
					photo := PhotoModel{}
					photo.Identifier = infos[l].Identifier
					photo.CreatedDate = infos[l].CreatedDate
					photo.Image = ""
					photoArray = append(photoArray, photo)
				}
			} else {
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
				photoArray[index].Image = "/Users/jinseonkim/go/src/hello/storage/" + part.FileName()
				index++
			}
		}

		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
		if err != nil {
			panic(err)
		}

		collection := client.Database("photocloud").Collection("photo")
		for i := range photoArray {
			filter := bson.M{"Identifier": photoArray[i].Identifier}
			update := bson.M{
				"$set": bson.M{"Image": photoArray[i].Image, "CreatedDate": photoArray[i].CreatedDate},
			}
			upsert := true
			after := options.After
			opt := options.FindOneAndUpdateOptions{
				ReturnDocument: &after,
				Upsert:         &upsert,
			}
			result := collection.FindOneAndUpdate(ctx, filter, update, &opt)
			if result.Err() != nil {
				panic(result.Err())
			}
		}
		//display success message.
		fmt.Fprint(w, "success")

	case "DELETE":
		respBody, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		str := string(respBody)
		bytes := []byte(str)
		var infos []Info
		json.Unmarshal(bytes, &infos)

		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
		if err != nil {
			panic(err)
		}
		collection := client.Database("photocloud").Collection("photo")
		for i := range infos {
			filter := bson.M{"Identifier": infos[i].Identifier}
			opt := options.FindOneAndDeleteOptions{}
			result := collection.FindOneAndDelete(ctx, filter, &opt)
			if result.Err() != nil {
				panic(result.Err())
			} else {

			}
		}
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
