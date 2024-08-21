// Package docs Code generated by swaggo/swag. DO NOT EDIT
package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {},
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/publish": {
            "post": {
                "description": "Publish a message to a specified topic",
                "consumes": [
                    "text/xml"
                ],
                "produces": [
                    "text/plain"
                ],
                "tags": [
                    "messaging"
                ],
                "summary": "Publish a message to a topic",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Topic",
                        "name": "topic",
                        "in": "query",
                        "required": true
                    },
                    {
                        "description": "Message Payload",
                        "name": "message",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/main.MessagePayload"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Message published",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "400": {
                        "description": "Invalid XML",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Failed to publish message",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/subscribe": {
            "get": {
                "description": "Subscribe to a specified topic and receive messages",
                "consumes": [
                    "text/plain"
                ],
                "produces": [
                    "text/xml"
                ],
                "tags": [
                    "messaging"
                ],
                "summary": "Subscribe to a topic",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Topic",
                        "name": "topic",
                        "in": "query",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Received message",
                        "schema": {
                            "$ref": "#/definitions/main.MessagePayload"
                        }
                    },
                    "500": {
                        "description": "Failed to receive message",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "main.MessagePayload": {
            "type": "object",
            "properties": {
                "content": {
                    "type": "string"
                },
                "sender": {
                    "type": "string"
                }
            }
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "",
	Host:             "",
	BasePath:         "",
	Schemes:          []string{},
	Title:            "",
	Description:      "",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
