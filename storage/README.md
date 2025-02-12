# storage

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

A number of features are provided out of the box.

Storage File features such as

- Read a File
- Write content to a file
- List all the files of a bucket/folder
- Get information about a file
- Add metadata to a file
- Read metadat value of a file
- Delete a file

Storage File System features such as

- Create a file, folder or a bucket
- Open a file in a given location

## Usage

Setup the SQS library in order to start using it.
Under you main pacakge, you can add an init function or any method of your choice to initiate the library

The Priority of the Registered Provider is as follows

```bash
URL > HOST > Scheme("storage") > default
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
    gcpsvc.Manager.Register("storage", config)
}
```

## URL Format to use

```bash
storage://bucketName/folderName.../fileName
```

## Examples

1. Create a bucket/file

    ```go
    package main
   
    import (
        _ "oss.nandlabs.io/golly-gcp/storage"
        "oss.nandlabs.io/golly/vfs"   
    )
   
    func main() {
        manager := vfs.GetManager()
        u, err := url.Parse("storage://bucketName")
        if err != nil {
            // handle error
        }
        file, err := manager.Create(u)

        if err != nil {
            // handle error
        }
        fmt.Println(file.Info())
    }
    ```

2. Read a file
3. Delete a file
4. Write a file
5. List all the files in the bucket
6. Get File Info of an object
7. Get metadata of an object
8. Add metadata to an object

## Contributing

We welcome contributions to the SQS library! If you find a bug, have a feature request, or want to contribute improvements, please create a pull request. For major changes, please open an issue first to discuss the changes you would like to make.
