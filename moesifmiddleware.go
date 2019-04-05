/*
 * moesifmiddleware-go
 */
package moesifmiddleware

import (
  "log"
	"net/http"
	"bytes"
	"io"
	moesifapi "github.com/moesif/moesifapi-go"
	"github.com/moesif/moesifapi-go/models"
	"time"
  "fmt"
	"crypto/rand"
	"io/ioutil"
	"encoding/json"
)

// Global variable
var (
	apiClient moesifapi.API
	debug bool
)

// Initialize the client
func moesifClient(moesifOption map[string]interface{}) {
	api := moesifapi.NewAPI(moesifOption["Application_Id"].(string))
	apiClient = api
}

// Moesif Response Recorder
type MoesifResponseRecorder struct {
	rw http.ResponseWriter
	status int
	writer io.Writer
	header map[string][]string
}

// Function to generate UUID
func uuid() (string, error) {
 b := make([]byte, 16)
 _, err := rand.Read(b)

 if err != nil {
	 return "", err
 }
 return fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// Response Recorder
func responseRecorder(rw http.ResponseWriter, status int, writer io.Writer) MoesifResponseRecorder{
	rr := MoesifResponseRecorder{
		rw,
		status,
		writer,
		make(map[string][]string, 5),
	}
	return rr
}

// Implementing the WriteHeader method of ResponseWriter Interface
func (rec *MoesifResponseRecorder) WriteHeader(code int) {
	rec.status = code
	rec.rw.WriteHeader(code)
}

// Implementing the Write method of ResponseWriter Interface
func (rec *MoesifResponseRecorder) Write(b []byte) (int, error){
	return rec.writer.Write(b)
}

// Implementing the Header method of ResponseWriter Interface
func (rec *MoesifResponseRecorder) Header() http.Header{
	return rec.rw.Header()
}

// Moesif Middleware
func MoesifMiddleware(next http.Handler, moesifOption map[string]interface{}) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, request *http.Request) {
		// Buffer
		var buf bytes.Buffer

		// Create a writer to duplicates it's writes to all the provided writers
		multiWriter := io.MultiWriter(rw, &buf)

		// Initialize the status to 200 in case WriteHeader is not called
		response := responseRecorder(
			rw,
			200,
			multiWriter,
		)

	 // Disable TransactionId by default
	 disableTransactionId := false
	 // Try to fetch the disableTransactionId from the option
	 if isEnabled, found := moesifOption["disableTransactionId"].(bool); found {
		 disableTransactionId = isEnabled
	 }

	 // Add transactionId to the headers
	 if !disableTransactionId {
		 // Try to fetch the transactionId from the header
		 transactionId := request.Header.Get("X-Moesif-Transaction-Id")
		 // Check if need to generate transactionId
		 if len(transactionId) == 0 {
			 transactionId, _ = uuid()
		 }

		 if len(transactionId) != 0 {
			 // Add transationId to the request header
			 request.Header.Set("X-Moesif-Transaction-Id", transactionId)

			 // Add transationId to the response header
			 rw.Header().Add("X-Moesif-Transaction-Id", transactionId)
		 }
	 }

		// Request Time
		requestTime := time.Now().UTC()

		// Serve the HTTP Request
		next.ServeHTTP(&response, request)

		// Response Time
		responseTime := time.Now().UTC()

		// Call the function to initialize the moesif client
		if apiClient == nil {
		 moesifClient(moesifOption)
		}

	 debug = false
	 if isDebug, found := moesifOption["Debug"].(bool); found {
			 debug = isDebug
	 }

	 shouldSkip := false
	 if _, found := moesifOption["Should_Skip"]; found {
		 shouldSkip = moesifOption["Should_Skip"].(func(*http.Request, MoesifResponseRecorder) bool)(request, response)
	 }

	 if shouldSkip {
		 if debug{
			 log.Printf("Skip sending the event to Moesif")
		 }
	 } else {
		 if debug {
			 log.Printf("Sending the event to Moesif")
		 }
		 // Call the function to send event to Moesif
		 sendEvent(request, response, buf.String(), requestTime, responseTime, moesifOption)
	 }
	})
}

// Sending event to Moesif
func sendEvent(request *http.Request, response MoesifResponseRecorder, rspBody string, reqTime time.Time, rspTime time.Time, moesifOption map[string]interface{}) {
	// Get Api Version
	var apiVersion *string = nil
	if isApiVersion, found := moesifOption["Api_Version"].(string); found {
		apiVersion = &isApiVersion
	}

	// Get Request Body
	var reqBody interface{}
	readReqBody, reqBodyErr := ioutil.ReadAll(request.Body)
	if reqBodyErr != nil {
		log.Printf("Error while reading request body: %s.\n", reqBodyErr.Error())
	}

	if jsonMarshalErr := json.Unmarshal(readReqBody, &reqBody); jsonMarshalErr != nil {
		log.Printf("Error while parsing request body: %s.\n", jsonMarshalErr.Error())
		reqBody = nil
	}

	// Get URL Scheme
	if request.URL.Scheme == "" {
		request.URL.Scheme = "http"
	}

	// Get Metadata
	var metadata map[string]interface{} = nil
	if _, found := moesifOption["Get_Metadata"]; found {
		metadata = moesifOption["Get_Metadata"].(func(*http.Request, MoesifResponseRecorder) map[string]interface{})(request, response)
	}

	// Get User
	var userId string
	if _, found := moesifOption["Identify_User"]; found {
		userId = moesifOption["Identify_User"].(func(*http.Request, MoesifResponseRecorder) string)(request, response)
	}

	// Get Session Token
	var sessionToken string
	if _, found := moesifOption["Get_Session_Token"]; found {
		sessionToken = moesifOption["Get_Session_Token"].(func(*http.Request, MoesifResponseRecorder) string)(request, response)
	}

	// Prepare request model
	event_request := models.EventRequestModel{
		Time:       &reqTime,
		Uri:        request.URL.Scheme + "://" + request.Host + request.URL.Path,
		Verb:       request.Method,
		ApiVersion: apiVersion,
		IpAddress:  &request.RemoteAddr,
		Headers:    request.Header,
		Body: 		 &reqBody,
	}

	// Prepare response model
	event_response := models.EventResponseModel{
		Time:      &rspTime,
		Status:    response.status,
		IpAddress: nil,
		Headers:   response.Header(),
		Body: 	   rspBody,
	}

	// Prepare the event model
	event := models.EventModel{
		Request:      event_request,
		Response:     event_response,
		SessionToken: &sessionToken,
		Tags:         nil,
		UserId:       &userId,
		CompanyId: 		nil,
		Metadata: 	   metadata,
	}

	// Execute mask event model function
	if _, found := moesifOption["Mask_Event_Model"]; found {
	 fmt.Printf("Event.\n%#v", event)
	 event = moesifOption["Mask_Event_Model"].(func(models.EventModel) models.EventModel)(event)
	 fmt.Printf("MAsked Event.\n%#v", event)
	}

	// Add event to the queue
	err := apiClient.QueueEvent(&event)

	// Log the message
	if err != nil {
		log.Fatalf("Error while adding event to Moesif: %s.\n", err.Error())
	} else {
	 if debug{
		 log.Println("Event successfully added to the queue")
	 }
	}
}
