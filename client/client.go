package client

import (
	"github.com/gorilla/websocket"
	"net/http"
)

type GenericClient struct {
	GenericBaseClient
	Publish PublishOperations
}

const (
	PUBLISH_TYPE = "publish"
)

type GenericBaseClient interface {
	Websocket(string, map[string][]string) (*websocket.Conn, *http.Response, error)
	List(string, *ListOpts, interface{}) error
	Post(string, interface{}, interface{}) error
	GetLink(Resource, string, interface{}) error
	Create(string, interface{}, interface{}) error
	Update(string, *Resource, interface{}, interface{}) error
	ById(string, string, interface{}) error
	Delete(*Resource) error
	Reload(*Resource, interface{}) error
	Action(string, string, *Resource, interface{}, interface{}) error
	GetOpts() *ClientOpts
	GetSchemas() *Schemas
	GetTypes() map[string]Schema

	doGet(string, *ListOpts, interface{}) error
	doList(string, *ListOpts, interface{}) error
	doNext(string, interface{}) error
	doModify(string, string, interface{}, interface{}) error
	doCreate(string, interface{}, interface{}) error
	doUpdate(string, *Resource, interface{}, interface{}) error
	doById(string, string, interface{}) error
	doResourceDelete(string, *Resource) error
	doAction(string, string, *Resource, interface{}, interface{}) error
}

type Publish struct {
	Resource

	Data map[string]interface{} `json:"data,omitempty" yaml:"data,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	PreviousIds []string `json:"previousIds,omitempty" yaml:"previous_ids,omitempty"`

	ResourceId string `json:"resourceId,omitempty" yaml:"resource_id,omitempty"`

	ResourceType string `json:"resourceType,omitempty" yaml:"resource_type,omitempty"`

	Time int64 `json:"time,omitempty" yaml:"time,omitempty"`

	Transitioning string `json:"transitioning,omitempty" yaml:"transitioning,omitempty"`

	TransitioningMessage string `json:"transitioningMessage,omitempty" yaml:"transitioning_message,omitempty"`
}

type PublishCollection struct {
	Collection
	Data   []Publish `json:"data,omitempty"`
	client *PublishClient
}

type PublishClient struct {
	apiClient *GenericClient
}

type PublishOperations interface {
	List(opts *ListOpts) (*PublishCollection, error)
	Create(opts *Publish) (*Publish, error)
	Update(existing *Publish, updates interface{}) (*Publish, error)
	ById(id string) (*Publish, error)
	Delete(container *Publish) error
}

func newPublishClient(apiClient *GenericClient) *PublishClient {
	return &PublishClient{
		apiClient: apiClient,
	}
}

func (c *PublishClient) Create(container *Publish) (*Publish, error) {
	resp := &Publish{}
	err := c.apiClient.doCreate(PUBLISH_TYPE, container, resp)
	return resp, err
}

func (c *PublishClient) Update(existing *Publish, updates interface{}) (*Publish, error) {
	resp := &Publish{}
	err := c.apiClient.doUpdate(PUBLISH_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *PublishClient) List(opts *ListOpts) (*PublishCollection, error) {
	resp := &PublishCollection{}
	err := c.apiClient.doList(PUBLISH_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *PublishCollection) Next() (*PublishCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &PublishCollection{}
		err := cc.client.apiClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *PublishClient) ById(id string) (*Publish, error) {
	resp := &Publish{}
	err := c.apiClient.doById(PUBLISH_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *PublishClient) Delete(container *Publish) error {
	return c.apiClient.doResourceDelete(PUBLISH_TYPE, &container.Resource)
}
