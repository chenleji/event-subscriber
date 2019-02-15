package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
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

type clientImpl struct {
	Opts    *ClientOpts
	Schemas *Schemas
	Types   map[string]Schema
}

func (apiClient *clientImpl) setupRequest(req *http.Request) {
	req.SetBasicAuth(apiClient.Opts.SecretID, apiClient.Opts.SecretKey)
}

func (apiClient *clientImpl) newHttpClient() *http.Client {
	if apiClient.Opts.Timeout == 0 {
		apiClient.Opts.Timeout = time.Second * 10
	}
	return &http.Client{Timeout: apiClient.Opts.Timeout}
}


func (apiClient *clientImpl) doGet(url string, opts *ListOpts, respObject interface{}) error {
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

func (apiClient *clientImpl) doList(schemaType string, opts *ListOpts, respObject interface{}) error {
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

func (apiClient *clientImpl) doNext(nextUrl string, respObject interface{}) error {
	return apiClient.doGet(nextUrl, nil, respObject)
}

func (apiClient *clientImpl) doModify(method string, url string, createObj interface{}, respObject interface{}) error {
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

func (apiClient *clientImpl) doCreate(schemaType string, createObj interface{}, respObject interface{}) error {
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

func (apiClient *clientImpl) doUpdate(schemaType string, existing *Resource, updates interface{}, respObject interface{}) error {
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

func (apiClient *clientImpl) doById(schemaType string, id string, respObject interface{}) error {
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

func (apiClient *clientImpl) doResourceDelete(schemaType string, existing *Resource) error {
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

func (apiClient *clientImpl) doAction(schemaType string, action string,
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


func (apiClient *clientImpl) doDelete(url string) error {
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
