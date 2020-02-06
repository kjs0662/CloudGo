package main


/*
#cgo CFLAGS: -I /Users/jinseonkim/go/src/libs/thumbnailer
#cgo LDFLAGS: -L/Users/jinseonkim/go/src/libs/thumbnailer -lthumbnailer
#cgo LDFLAGS: -L/usr/local/Cellar/opencv@2/2.4.13.7_7/lib -lopencv_calib3d -lopencv_contrib -lopencv_core -lopencv_features2d -lopencv_flann -lopencv_gpu -lopencv_highgui -lopencv_imgproc -lopencv_legacy -lopencv_ml -lopencv_nonfree -lopencv_objdetect -lopencv_ocl -lopencv_photo -lopencv_stitching -lopencv_superres -lopencv_ts -lopencv_video -lopencv_videostab

#include <stdlib.h>
#include <imageCompressor.h>
*/
import "C"
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"
	"unsafe"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
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
	Name        string
}

type PhotoModel struct {
	Identifier  string
	Image       string
	Thumbnail   string
	CreatedDate string
	Name        string
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	//GET displays the upload form.
	case "GET":
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
		if err != nil {
			panic(err)
		}
		collection := client.Database("cloud").Collection("photo")
		cursor, err := collection.Find(ctx, bson.M{})
		if err != nil {
			fmt.Print("error is here\n")
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

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
		if err != nil {
			println("Error occur to connect mongodb")
			panic(err)
		}
		collection := client.Database("cloud").Collection("photo")

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
					photo.Thumbnail = ""
					photo.Name = ""
					photoArray = append(photoArray, photo)
				}
			} else {
				sess, err := session.NewSession(&aws.Config{
					Region: aws.String("ap-northeast-1"),
					Credentials: credentials.NewStaticCredentials(
						"AKIARI7I35SVOYOCQOYH",
						"8R/rimRYf1IEhDHHxdupgVw4Q6D3QFmXBlMAc2vR",
						""),
				})
				if err != nil {
					panic(err)
				}

				dst, err := os.Create("/home/ec2-user/go/cloud/storage/" + part.FileName())
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

				path := "/home/ec2-user/go/cloud/storage/" + part.FileName()
				photoArray[index].Name = path

				filter := bson.M{"Identifier": photoArray[index].Identifier}
				upsert := true
				after := options.After
				findOpt := options.FindOneOptions{}
				findResult := collection.FindOne(ctx, filter, &findOpt)
				if findResult.Err() != nil {
					if findResult.Err().Error() == "mongo: no documents in result" {

						// upload image to s3
						uploader := s3manager.NewUploader(sess)
						file, err := os.Open(path)
						defer file.Close()
						if err != nil {
							println("Error occur to open tmp local data")
							panic(err)
						}
						result, err := uploader.Upload(&s3manager.UploadInput{
							Bucket: aws.String("jinseon-photo-bucket"),
							Key:    aws.String(part.FileName()),
							Body:   file,
						})
						if err != nil {
							os.Remove(path)
							println("Error occur to upload data to s3")
							panic(err)
						}
						// make thumbnail image
						img, _, err := image.Decode(file)
						if err != nil {
							panic(err)
						}
						size := img.Bounds().Size()
						rect := image.Rect(0, 0, size.X, size.Y)
						rgba := image.NewRGBA(rect)
						draw.Draw(rgba, rect, img, rect.Min, draw.Src)

						imageData := (*C.char)(unsafe.Pointer(&rgba.Pix[0]))
						width := C.int(size.X)
						height := C.int(size.Y)

						var compressedData *C.char
						compressedData = C.compressedImage(imageData, width, height)

						cSize := width * height * 4
						gSlice := (*[1 << 30]uint8)(unsafe.Pointer(compressedData))[:cSize:cSize]
						nImg := image.NewRGBA(rect)
						copy(nImg.Pix, gSlice)

						// change image to compressed jpeg
						thumbnailPath := "/home/ec2-user/go/cloud/storage/thumbanil-" + part.FileName()
						thumbnailFile, err := os.Create(thumbnailPath)
						err = jpeg.Encode(thumbnailFile, nImg, nil)
						if err != nil {
							println("Error occur to write image file to jpeg")
							panic(err)
						}
						thumbnailFile.Close()

						tmpFile, err := os.Open(thumbnailPath)
						defer tmpFile.Close()
						if err != nil {
							panic(err)
						}
						thumbResult, err := uploader.Upload(&s3manager.UploadInput{
							Bucket: aws.String("jinseon-thumbnail-bucket"),
							Key:    aws.String("thumbnail-" + part.FileName()),
							Body:   tmpFile,
						})
						if err != nil {
							os.Remove(path)
							os.Remove(thumbnailPath)
							println("Error occur to upload data to s3")
							panic(err)
						}
						os.Remove(path)
						os.Remove(thumbnailPath)
						photoArray[index].Image = result.Location
						photoArray[index].Thumbnail = thumbResult.Location
					}
				}

				// save to mongodb
				var update bson.M
				if photoArray[index].Image == "" {
					update = bson.M{
						"$set": bson.M{
							"CreatedDate": photoArray[index].CreatedDate,
							"Identifier":  photoArray[index].Identifier,
							"Name":        photoArray[index].Name},
					}
				} else {
					update = bson.M{
						"$set": bson.M{"Image": photoArray[index].Image,
							"Thumbnail":   photoArray[index].Thumbnail,
							"CreatedDate": photoArray[index].CreatedDate,
							"Identifier":  photoArray[index].Identifier,
							"Name":        photoArray[index].Name},
					}
				}
				opt := options.FindOneAndUpdateOptions{
					ReturnDocument: &after,
					Upsert:         &upsert,
				}
				dbResult := collection.FindOneAndUpdate(ctx, filter, update, &opt)
				if dbResult.Err() != nil {
					deleteS3(sess, "jinseon-photo-bucket", part.FileName())
					println("Error occur to update data to mongo")
					panic(dbResult.Err())
				}
				index++
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

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
		if err != nil {
			panic(err)
		}
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String("ap-northeast-1"),
			Credentials: credentials.NewStaticCredentials(
				"AKIARI7I35SVOYOCQOYH",
				"8R/rimRYf1IEhDHHxdupgVw4Q6D3QFmXBlMAc2vR",
				""),
		})
		if err != nil {
			panic(err)
		}
		collection := client.Database("cloud").Collection("photo")
		for i := range infos {
			filter := bson.M{"Identifier": infos[i].Identifier}
			opt := options.FindOneAndDeleteOptions{}
			result := collection.FindOneAndDelete(ctx, filter, &opt)
			if result.Err() != nil {
				panic(result.Err())
			} else {
				deleteS3(sess, "jinseon-photo-bucket", infos[i].Name)
			}
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func deleteS3(sess *session.Session, bucket string, obj string) {
	svc := s3.New(sess)
	_, err := svc.DeleteObject(&s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(obj)})
	if err != nil {
		fmt.Printf("Unable to delete object %q from bucket %q, %v", obj, bucket, err)
		panic(err)
	}

	err = svc.WaitUntilObjectNotExists(&s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(obj),
	})
}

func main() {

	http.HandleFunc("/", uploadHandler)

	//static file handler.
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	//Listen on port 9090
	http.ListenAndServe(":9090", nil)
}
