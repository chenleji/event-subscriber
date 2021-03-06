package client

import (
	"bytes"
	"encoding/json"
	"github.com/pkg/errors"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"
)

const (
	SELF       = "self"
	COLLECTION = "collection"
)

var (
	debug             = false
	dialer            = &websocket.Dialer{}
	privateFieldRegex = regexp.MustCompile("^[[:lower:]]")
)

type GenericBaseClientImpl struct {
	Opts    *ClientOpts
	Schemas *Schemas
	Types   map[string]Schema
} 

func (apiClient *GenericBaseClientImpl) setupRequest(req *http.Request) {
	req.SetBasicAuth(apiClient.Opts.SecretID, apiClient.Opts.SecretKey)
}

func (apiClient *GenericBaseClientImpl) newHttpClient() *http.Client {
	if apiClient.Opts.Timeout == 0 {
		apiClient.Opts.Timeout = time.Second * 10
	}
	return &http.Client{Timeout: apiClient.Opts.Timeout}
}

func (apiClient *GenericBaseClientImpl) doDelete(url string) error {
	client := apiClient.newHttpClient()
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	apiClient.setupRequest(req)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	io.Copy(ioutil.Discard, resp.Body)

	if resp.StatusCode >= 300 {
		return newApiError(resp, url)
	}

	return nil
}

func (apiClient *GenericBaseClientImpl) Websocket(url string, headers map[string][]string) (*websocket.Conn, *http.Response, error) {
	return dialer.Dial(url, http.Header(headers))
}

func (apiClient *GenericBaseClientImpl) doGet(url string, opts *ListOpts, respObject interface{}) error {
	if opts == nil {
		opts = NewListOpts()
	}
	url, err := appendFilters(url, opts.Filters)
	if err != nil {
		return err
	}

	if debug {
		fmt.Println("GET " + url)
	}

	client := apiClient.newHttpClient()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	apiClient.setupRequest(req)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return newApiError(resp, url)
	}

	byteContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if debug {
		fmt.Println("Response <= " + string(byteContent))
	}

	if err := json.Unmarshal(byteContent, respObject); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to parse: %s", byteContent))
	}

	return nil
}

func (apiClient *GenericBaseClientImpl) List(schemaType string, opts *ListOpts, respObject interface{}) error {
	return apiClient.doList(schemaType, opts, respObject)
}

func (apiClient *GenericBaseClientImpl) doList(schemaType string, opts *ListOpts, respObject interface{}) error {
	schema, ok := apiClient.Types[schemaType]
	if !ok {
		return errors.New("Unknown schema type [" + schemaType + "]")
	}

	if !contains(schema.CollectionMethods, "GET") {
		return errors.New("Resource type [" + schemaType + "] is not listable")
	}

	collectionUrl, ok := schema.Links[COLLECTION]
	if !ok {
		return errors.New("Failed to find collection URL for [" + schemaType + "]")
	}

	return apiClient.doGet(collectionUrl, opts, respObject)
}

func (apiClient *GenericBaseClientImpl) doNext(nextUrl string, respObject interface{}) error {
	return apiClient.doGet(nextUrl, nil, respObject)
}

func (apiClient *GenericBaseClientImpl) Post(url string, createObj interface{}, respObject interface{}) error {
	return apiClient.doModify("POST", url, createObj, respObject)
}

func (apiClient *GenericBaseClientImpl) GetLink(resource Resource, link string, respObject interface{}) error {
	url := resource.Links[link]
	if url == "" {
		return fmt.Errorf("Failed to find link: %s", link)
	}

	return apiClient.doGet(url, &ListOpts{}, respObject)
}

func (apiClient *GenericBaseClientImpl) doModify(method string, url string, createObj interface{}, respObject interface{}) error {
	bodyContent, err := json.Marshal(createObj)
	if err != nil {
		return err
	}

	if debug {
		fmt.Println(method + " " + url)
		fmt.Println("Request => " + string(bodyContent))
	}

	client := apiClient.newHttpClient()
	req, err := http.NewRequest(method, url, bytes.NewBuffer(bodyContent))
	if err != nil {
		return err
	}

	apiClient.setupRequest(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return newApiError(resp, url)
	}

	byteContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if len(byteContent) > 0 {
		if debug {
			fmt.Println("Response <= " + string(byteContent))
		}
		return json.Unmarshal(byteContent, respObject)
	}

	return nil
}

func (apiClient *GenericBaseClientImpl) Create(schemaType string, createObj interface{}, respObject interface{}) error {
	return apiClient.doCreate(schemaType, createObj, respObject)
}

func (apiClient *GenericBaseClientImpl) doCreate(schemaType string, createObj interface{}, respObject interface{}) error {
	if createObj == nil {
		createObj = map[string]string{}
	}
	if respObject == nil {
		respObject = &map[string]interface{}{}
	}
	schema, ok := apiClient.Types[schemaType]
	if !ok {
		return errors.New("Unknown schema type [" + schemaType + "]")
	}

	if !contains(schema.CollectionMethods, "POST") {
		return errors.New("Resource type [" + schemaType + "] is not creatable")
	}

	var collectionUrl string
	collectionUrl, ok = schema.Links[COLLECTION]
	if !ok {
		// return errors.New("Failed to find collection URL for [" + schemaType + "]")
		// This is a hack to address https://github.com/rancher/cattle/issues/254
		re := regexp.MustCompile("schemas.*")
		collectionUrl = re.ReplaceAllString(schema.Links[SELF], schema.PluralName)
	}

	return apiClient.doModify("POST", collectionUrl, createObj, respObject)
}

func (apiClient *GenericBaseClientImpl) Update(schemaType string, existing *Resource, updates interface{}, respObject interface{}) error {
	return apiClient.doUpdate(schemaType, existing, updates, respObject)
}

func (apiClient *GenericBaseClientImpl) doUpdate(schemaType string, existing *Resource, updates interface{}, respObject interface{}) error {
	if existing == nil {
		return errors.New("Existing object is nil")
	}

	selfUrl, ok := existing.Links[SELF]
	if !ok {
		return errors.New(fmt.Sprintf("Failed to find self URL of [%v]", existing))
	}

	if updates == nil {
		updates = map[string]string{}
	}

	if respObject == nil {
		respObject = &map[string]interface{}{}
	}

	schema, ok := apiClient.Types[schemaType]
	if !ok {
		return errors.New("Unknown schema type [" + schemaType + "]")
	}

	if !contains(schema.ResourceMethods, "PUT") {
		return errors.New("Resource type [" + schemaType + "] is not updatable")
	}

	return apiClient.doModify("PUT", selfUrl, updates, respObject)
}

func (apiClient *GenericBaseClientImpl) ById(schemaType string, id string, respObject interface{}) error {
	return apiClient.doById(schemaType, id, respObject)
}

func (apiClient *GenericBaseClientImpl) doById(schemaType string, id string, respObject interface{}) error {
	schema, ok := apiClient.Types[schemaType]
	if !ok {
		return errors.New("Unknown schema type [" + schemaType + "]")
	}

	if !contains(schema.ResourceMethods, "GET") {
		return errors.New("Resource type [" + schemaType + "] can not be looked up by ID")
	}

	collectionUrl, ok := schema.Links[COLLECTION]
	if !ok {
		return errors.New("Failed to find collection URL for [" + schemaType + "]")
	}

	err := apiClient.doGet(collectionUrl+"/"+id, nil, respObject)
	//TODO check for 404 and return nil, nil
	return err
}

func (apiClient *GenericBaseClientImpl) Delete(existing *Resource) error {
	if existing == nil {
		return nil
	}
	return apiClient.doResourceDelete(existing.Type, existing)
}

func (apiClient *GenericBaseClientImpl) doResourceDelete(schemaType string, existing *Resource) error {
	schema, ok := apiClient.Types[schemaType]
	if !ok {
		return errors.New("Unknown schema type [" + schemaType + "]")
	}

	if !contains(schema.ResourceMethods, "DELETE") {
		return errors.New("Resource type [" + schemaType + "] can not be deleted")
	}

	selfUrl, ok := existing.Links[SELF]
	if !ok {
		return errors.New(fmt.Sprintf("Failed to find self URL of [%v]", existing))
	}

	return apiClient.doDelete(selfUrl)
}

func (apiClient *GenericBaseClientImpl) Reload(existing *Resource, output interface{}) error {
	selfUrl, ok := existing.Links[SELF]
	if !ok {
		return errors.New(fmt.Sprintf("Failed to find self URL of [%v]", existing))
	}

	return apiClient.doGet(selfUrl, NewListOpts(), output)
}

func (apiClient *GenericBaseClientImpl) Action(schemaType string, action string,
	existing *Resource, inputObject, respObject interface{}) error {
	return apiClient.doAction(schemaType, action, existing, inputObject, respObject)
}

func (apiClient *GenericBaseClientImpl) doAction(schemaType string, action string,
	existing *Resource, inputObject, respObject interface{}) error {

	if existing == nil {
		return errors.New("Existing object is nil")
	}

	actionUrl, ok := existing.Actions[action]
	if !ok {
		return errors.New(fmt.Sprintf("Action [%v] not available on [%v]", action, existing))
	}

	_, ok = apiClient.Types[schemaType]
	if !ok {
		return errors.New("Unknown schema type [" + schemaType + "]")
	}

	var input io.Reader

	if inputObject != nil {
		bodyContent, err := json.Marshal(inputObject)
		if err != nil {
			return err
		}
		if debug {
			fmt.Println("Request => " + string(bodyContent))
		}
		input = bytes.NewBuffer(bodyContent)
	}

	client := apiClient.newHttpClient()
	req, err := http.NewRequest("POST", actionUrl, input)
	if err != nil {
		return err
	}

	apiClient.setupRequest(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", "0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return newApiError(resp, actionUrl)
	}

	byteContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if debug {
		fmt.Println("Response <= " + string(byteContent))
	}

	return json.Unmarshal(byteContent, respObject)
}

func (apiClient *GenericBaseClientImpl) GetOpts() *ClientOpts {
	return apiClient.Opts
}

func (apiClient *GenericBaseClientImpl) GetSchemas() *Schemas {
	return apiClient.Schemas
}

func (apiClient *GenericBaseClientImpl) GetTypes() map[string]Schema {
	return apiClient.Types
}

func init() {
	debug = os.Getenv("RANCHER_CLIENT_DEBUG") == "true"
	if debug {
		fmt.Println("Rancher client debug on")
	}
}

func contains(array []string, item string) bool {
	for _, check := range array {
		if check == item {
			return true
		}
	}

	return false
}

func appendFilters(urlString string, filters map[string]interface{}) (string, error) {
	if len(filters) == 0 {
		return urlString, nil
	}

	u, err := url.Parse(urlString)
	if err != nil {
		return "", err
	}

	q := u.Query()
	for k, v := range filters {
		if l, ok := v.([]string); ok {
			for _, v := range l {
				q.Add(k, v)
			}
		} else {
			q.Add(k, fmt.Sprintf("%v", v))
		}
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

func NewListOpts() *ListOpts {
	return &ListOpts{
		Filters: map[string]interface{}{},
	}
}