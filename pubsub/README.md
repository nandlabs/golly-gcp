# pubsub

This implementation provides you a set of standard functions to consume be the end user without worrying about any underlyig complexities.

---

- [Installation](#installation)
- [Features](#features)
- [Usage](#usage)
- [Examples](#examples)
- [Contributing](#contributing)

---

## Installation

```bash
go get oss.nandlabs.io/golly-gcp/storge
```

## Features

A number of features are provided out of the box

- Ability to send a message to PubSub
- Ability to send multiple messages to PubSub
- Ability to consume messages from PubSub
- Ability to consume multiple messages from PubSub

## Usage

Setup the SQS library in order to start using it.
Under you main pacakge, you can add an init function or any method of your choice to initiate the library

```go
package main

import (
    "context"

    "oss.nandlabs.io/golly-gcp/gcpsvc"
)

func init() {
    config := gcpsvc.Config{
        ProjectId: "project-id",
    }
    gcpsvc.Manager.Register("pubsub", config)
}
```

## URL Format to use

```bash
pubsub://topic-id
pubsub://subscription-id
```

## Examples

## Contributing

We welcome contributions to the SQS library! If you find a bug, have a feature request, or want to contribute improvements, please create a pull request. For major changes, please open an issue first to discuss the changes you would like to make.
