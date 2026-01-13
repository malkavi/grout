package romm

type Config struct {
	PlatformsBinding map[string]string `json:"PLATFORMS_BINDING"`
}

func (c *Client) GetConfig() (Config, error) {
	var config Config
	err := c.doRequest("GET", endpointConfig, nil, nil, &config)
	return config, err
}
