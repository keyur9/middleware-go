/*
 * moesifmiddleware-go
 */
package moesifmiddleware //main //

import (
    "log"
	"net/http"
	"bytes"
	"io"
	moesifapi "github.com/moesif/moesifapi-go"
	"github.com/moesif/moesifapi-go/models"
	"time"
	"encoding/json"
	// "strconv"
	// "strings"
)

var (
	apiClient moesifapi.API
)

// var moesifOption = map[interface{}]string {
// 	"Application_Id": "eyJhcHAiOiI1MTk6MTMxIiwidmVyIjoiMi4wIiwib3JnIjoiMTE2OjUzIiwiaWF0IjoxNTUwODgwMDAwfQ.bDqdFTQCXYH2oPrVHnqdUl3kVjml74f5aajy7cKDZew",
// }

// type Employee struct{
// 	DateOfBirth      *time.Time 		`json:"date_of_birth" form:"date_of_birth"`			//Time when request was made
// 	Id				 int				`json:"id" form:"id"`                               //HTTP Status code such as 200
// 	FirstName		 string				`json:"first_name" form:"first_name"`               //verb of the API request such as GET or POST
// 	LastName		 string				`json:"last_name" form:"last_name"`                 //verb of the API request such as GET or POST
// }

func moesifClient(moesifOption map[interface{}]string) {
	api := moesifapi.NewAPI(moesifOption["Application_Id"])
	apiClient = api
}

type moesifResponseRecorder struct {
	rw http.ResponseWriter
	status int
	writer io.Writer
	header map[string][]string
}

func responseRecorder(rw http.ResponseWriter, status int, writer io.Writer)  moesifResponseRecorder{
	rr := moesifResponseRecorder{
		rw,
		status,
		writer,
		make(map[string][]string, 5),
	}
	return rr
}

func (rec *moesifResponseRecorder) WriteHeader(code int) {
	rec.status = code
	rec.rw.WriteHeader(code)
}

func (rec *moesifResponseRecorder) Write(b []byte) (int, error){
	return rec.writer.Write(b)
}

func (rec *moesifResponseRecorder) Header() http.Header{
	return rec.rw.Header()
}

func MoesifMiddleware(next http.Handler, moesifOption map[interface{}]string) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, request *http.Request) {

		var buf bytes.Buffer
		multiWriter := io.MultiWriter(rw, &buf)

		// Initialize the status to 200 in case WriteHeader is not called
		response := responseRecorder(
			rw,
			200,
			multiWriter,
		)

		next.ServeHTTP(&response, request)

		// Call the function to initialize the moesif client
		moesifClient(moesifOption)

		// Call the function to send evnet to Moesif
		sendEvent(request, response, buf.String())

	})
}

// func ParseID(s string) (id int, err error) {
// 	p := strings.LastIndex(s, "/")
// 	if p < 0 {
// 		return 0, nil
// 	}

// 	first := s[:p+1]
// 	if first != "/api/employee/" {
// 		return 0, nil
// 	}

// 	id, err = strconv.Atoi(s[p+1:])
// 	if err != nil {
// 		return 0, nil
// 	}
// 	return id, nil
// }

// func main() {
// 	http.Handle("/api/employee/", moesifMiddleware(http.HandlerFunc(handle), moesifOption))
// 	err := http.ListenAndServe(":3000", nil)
// 	if err != nil {
// 		log.Fatalf("Could not start server: %s\n", err.Error())
// 	}
// }

// func handle(w http.ResponseWriter, r *http.Request) {
// 	w.WriteHeader(http.StatusOK)
// 	time := time.Now().UTC().AddDate(-30, 0, 0)
// 	id, _ := ParseID(r.URL.Path)
// 	var employee = Employee{
// 		DateOfBirth: &time,
// 		Id: id,
// 		FirstName: "firstName",
// 		LastName: "lastName",
// 	}
// 	w.Header().Set("Content-Type", "application/json")
// 	json.NewEncoder(w).Encode(employee)
// }

func UnMarshalJsonObject(rspBody string) (map[string]interface{}) {
	var jsonObject map[string]interface{}
	jsonerror := json.Unmarshal([]byte(rspBody), &jsonObject)
	if jsonerror != nil {
		log.Printf("Error while parsing Json Object: %s.\n", jsonerror.Error())
		return nil
	} else {
		return jsonObject
	}
}

func unmarshalJsonArray(rspBody string) ([]interface{}) {
	var jsonArray []interface{}

	jsonerror := json.Unmarshal([]byte(rspBody), &jsonArray)
	if jsonerror != nil {
		log.Printf("Error while parsing Json Array: %s.\n", jsonerror.Error())
		return nil
	} else {
		return jsonArray
	}
}

func sendEvent(request *http.Request, response moesifResponseRecorder, rspBody string) {

	reqTime := time.Now().UTC()

	event_request := models.EventRequestModel{
		Time:       &reqTime,
		Uri:        request.Host + request.RequestURI,
		Verb:       request.Method,
		ApiVersion: nil,
		IpAddress:  nil,
		Headers: request.Header,
		Body: nil,
		}
	
	rspTime := time.Now().UTC().Add(time.Duration(1) * time.Second)

	var event_response models.EventResponseModel

	// Try to parse the body
	var jsonArray = unmarshalJsonArray(rspBody)
	if jsonArray == nil {
		var jsonObject = UnMarshalJsonObject(rspBody)
		if jsonObject == nil {
			log.Printf("Body - default string")
			event_response = models.EventResponseModel{
				Time:      &rspTime,
				Status:    response.status,
				IpAddress: nil,
				Headers: response.Header(),
				Body: rspBody,
			}	
		} else {
			log.Printf("Body - json object")
			event_response = models.EventResponseModel{
				Time:      &rspTime,
				Status:    response.status,
				IpAddress: nil,
				Headers: response.Header(),
				Body: jsonObject,
			}
		}
	} else {
		log.Printf("Body - json array")
		event_response = models.EventResponseModel{
			Time:      &rspTime,
			Status:    response.status,
			IpAddress: nil,
			Headers: response.Header(),
			Body: jsonArray,
		}
	}
	
	event := models.EventModel{
		Request:      event_request,
		Response:     event_response,
		SessionToken: nil,
		Tags:         nil,
		UserId:       nil,
		Metadata: 	  nil,
	}

	err := apiClient.QueueEvent(&event)
	if err != nil {
		log.Fatalf("Error while adding event to Moesif: %s.\n", err.Error())
	}

	// log.Printf("Event.\n%#v", event)
	log.Println("Event successfully added to the queue")
}