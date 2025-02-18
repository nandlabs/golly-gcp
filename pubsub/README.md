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

The Priority of the Registered Provider is as follows

```bash
URL > HOST > Scheme("pubsub") > default
```

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

1. Send a message to PubSub

    ```go
    package main

    import (
        "net/url"

        _ "oss.nandlabs.io/golly-gcp/pubsub"
    )

    func main() {
        manager := messaging.GetManager()
        u, err := url.Parse("pubsub://topic-id")
        if err != nil {
            // handle error
        }
        message, err := manager.NewMessage(u.Scheme)
        if err != nil {
            // handle error
        }
        message.SetBodyStr("hello pubsub from golly")

        if err := manager.Send(u, message); err != nil {
            // handle error
        }
    } 
    ```

2. Send multiple messages to PubSub

    ```go
    package main

    import (
        "net/url"

        _ "oss.nandlabs.io/golly-gcp/pubsub"
    )
    
    func main() {
        manager := messaging.GetManager()
        u, err := url.Parse("pubsub://topic-id")
        if err != nil {
            fmt.Println(err)
        }
        var messages []*messaging.Message
        msg1, err := manager.NewMessage(u.Scheme)
        if err != nil {
            // handle error
        }
        msg1.SetBodyStr("this is message1")
        messages = append(messages, msg1)
        msg2, err := manager.NewMessage(u.Scheme)
        if err != nil {
            // handle error
        }
        msg2.SetBodyStr("this is message2")
        messages = append(messages, msg2)
        if err := manager.SendBatch(u, messages); err != nil {
            // handle error
        }
    }
    ```

3. Consume a message from PubSub

    ```go
    package main

    import (
        "net/url"

        _ "oss.nandlabs.io/golly-gcp/pubsub"
    )
    
    func main() {
        manager := messaging.GetManager()
        u, err := url.Parse("pubsub://subscription-id")
        if err != nil {
            fmt.Println(err)
        }
        optionsBuilder := messaging.NewOptionsBuilder()
        optionsBuilder.Add("Timeout", 10)
        options := optionsBuilder.Build()
        msg, err := manager.Receive(u, options...)
        if err != nil {
            // handle error
        }
        // handle received message (msg)
    }
    ```

4. Consume multiple messages from PubSub

    ```go
    package main

    import (
        "net/url"

        _ "oss.nandlabs.io/golly-gcp/pubsub"
    )
    
    func main() {
        manager := messaging.GetManager()
        u, err := url.Parse("pubsub://subscription-id")
        if err != nil {
            fmt.Println(err)
        }
        optionsBuilder := messaging.NewOptionsBuilder()
        optionsBuilder.Add("BatchSize", 5)
        optionsBuilder.Add("Timeout", 10)

        options := optionsBuilder.Build()
        msgs, err := manager.ReceiveBatch(u, options...)
        if err != nil {
            // handle error
        }
        for _, msg := range msgs {
            // handle received messages (msgs)
        }
    }
    ```

5. Add a listener to PubSub Subscription

    ```go
    package main

    import (
        "net/url"

        _ "oss.nandlabs.io/golly-gcp/pubsub"
    )
    
    func main() {
        manager := messaging.GetManager()
        u, err := url.Parse("pubsub://subscription-id")
        if err != nil {
            fmt.Println(err)
        }
        handler := func(msg messaging.Message) {
            fmt.Printf("Received message ID: %s\nBody: %s\n", msg.ID, msg.Body)
            // Add your message processing logic here
        }

        err := manager.AddListener(u, handler, messaging.Option{Key: "MaxMessages", Value: int32(5)}, messaging.Option{Key: "WaitTime", Value: int32(10)})
        if err != nil {
            // handle error
        }
    }
    ```

## Contributing

We welcome contributions to the SQS library! If you find a bug, have a feature request, or want to contribute improvements, please create a pull request. For major changes, please open an issue first to discuss the changes you would like to make.
