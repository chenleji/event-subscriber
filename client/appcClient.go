package client

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

func NewAppCClient(opts *ClientOpts) (*GenericClient, error) {
	genericBaseClient := &GenericBaseClientImpl{
		Types: map[string]Schema{},
	}
	cli := constructClient(genericBaseClient)

	err := setupGenericBaseClient(genericBaseClient, opts)
	if err != nil {
		return nil, err
	}

	return cli, nil
}

func constructClient(genericBaseClient *GenericBaseClientImpl) *GenericClient {
	client := &GenericClient{
		GenericBaseClient: genericBaseClient,
	}

	client.Publish = newPublishClient(client)

	return client
}

func setupGenericBaseClient(rancherClient *GenericBaseClientImpl, opts *ClientOpts) error {
	if opts.Timeout == 0 {
		opts.Timeout = time.Second * 10
	}
	client := &http.Client{Timeout: opts.Timeout}
	req, err := http.NewRequest("GET", opts.Url, nil)
	if err != nil {
		return err
	}

	req.SetBasicAuth(opts.SecretID, opts.SecretKey)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return newApiError(resp, opts.Url)
	}

	schemasUrls := resp.Header.Get("X-API-Schemas")
	if len(schemasUrls) == 0 {
		return errors.New("Failed to find schema at [" + opts.Url + "]")
	}

	if schemasUrls != opts.Url {
		req, err = http.NewRequest("GET", schemasUrls, nil)
		req.SetBasicAuth(opts.SecretID, opts.SecretKey)
		if err != nil {
			return err
		}

		resp, err = client.Do(req)
		if err != nil {
			return err
		}

		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return newApiError(resp, opts.Url)
		}
	}

	var schemas Schemas
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(bytes, &schemas)
	if err != nil {
		return err
	}

	rancherClient.Opts = opts
	rancherClient.Schemas = &schemas

	for _, schema := range schemas.Data {
		rancherClient.Types[schema.Id] = schema
	}

	return nil
}
