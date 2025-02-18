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

- Create a file or a folder
- Open a file in a given location

## Usage

Setup the storage library in order to start using it.
Under you main pacakge, you can add an init function or any method of your choice to initiate the library

The Priority of the Registered Provider is as follows

```bash
URL > HOST > Scheme("gs") > default
```

```go
package main

import (
    "context"

    "oss.nandlabs.io/golly-gcp/gcpsvc"
)

func init() {
    credentialsPath := "{enter the path of the credentials}"
    config := gcpsvc.Config{
        ProjectId: "project-id",
    }
    config.SetCredentialFile(credentialsPath)
    gcpsvc.Manager.Register("gs", config)
}
```

## URL Format to use

```bash
gs://bucketName/folderName.../fileName
```

## Examples

1. Create a file

   ```go
   package main

   import (
       _ "oss.nandlabs.io/golly-gcp/storage"
       "oss.nandlabs.io/golly/vfs"
   )

   func main() {
       manager := vfs.GetManager()
       u, err := url.Parse("gs://bucketName")
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

2. Create a folder

   ```go
   package main

   import (
       _ "oss.nandlabs.io/golly-gcp/storage"
       "oss.nandlabs.io/golly/vfs"
   )

   func main() {
       manager := vfs.GetManager()
       fmt.Printf("%v\n", manager)
       #
       u, err := url.Parse("gs://{bucket_name}/folder_1/")
       fmt.Println(u)
       if err != nil {
           // handle error
           fmt.Println(err)
           return
       }
       resp, err := manager.Create(u)
       if err != nil {
           fmt.Println(err)
           return
       }
       fmt.Println(resp)
   }
   ```

3. Read a file

   ```go
   package main

   import (
       _ "oss.nandlabs.io/golly-gcp/storage"
       "oss.nandlabs.io/golly/vfs"
   )

   func main() {
       manager := vfs.GetManager()
       u, err := url.Parse("gs://{bucket_name}/folder_1/gopher-image.png")
       if err != nil {
           fmt.Println(err)
           return
       }
       file, err := manager.Open(u)
       if err != nil {
           fmt.Println(err)
           return
       }
       buffer := make([]byte, 1024)
       n, err := file.Read(buffer)
       if err != nil {
           fmt.Println(err)
           return
       }
       fmt.Println(n)
   }
   ```

4. Delete a file

   ```go
   package main

   import (
       _ "oss.nandlabs.io/golly-gcp/storage"
       "oss.nandlabs.io/golly/vfs"
   )

   func main() {
       manager := vfs.GetManager()
       // folder1 - was a file
       u, err := url.Parse("gs://golly-test-app/folder1")
       if err != nil {
           fmt.Println(err)
           return
       }
       file, err := manager.Open(u)
       if err != nil {
           fmt.Println(err)
           return
       }
       err = file.Delete()
       if err != nil {
           fmt.Println(err)
           return
       }
   }
   ```

5. Write a file

   ```go
   package main

   import (
       _ "oss.nandlabs.io/golly-gcp/storage"
       "oss.nandlabs.io/golly/vfs"
   )

   func main() {

   }
   ```

6. List all the files in the bucket
7. Get File Info of an object

   ```go
   package main

   import (
       _ "oss.nandlabs.io/golly-gcp/storage"
       "oss.nandlabs.io/golly/vfs"
   )

   func main() {
       manager := vfs.GetManager()
       u, err := url.Parse("gs://{bucket_name}/folder_1/gopher-image.png")
       if err != nil {
           fmt.Println(err)
           return
       }
       file, err := manager.Open(u)
       if err != nil {
           fmt.Println(err)
           return
       }
       info, err := file.Info()
       if err != nil {
           fmt.Println(err)
           return
       }
       fmt.Println(info)
   }
   ```

8. Get metadata of an object

   ```go
   package main

   import (
       _ "oss.nandlabs.io/golly-gcp/storage"
       "oss.nandlabs.io/golly/vfs"
   )

   func main() {
       manager := vfs.GetManager()
       u, err := url.Parse("gs://{bucket_name}/folder_1/gopher-image.png")
       if err != nil {
           fmt.Println(err)
           return
       }
       file, err := manager.Open(u)
       if err != nil {
           fmt.Println(err)
           return
       }
       val, err := file.GetProperty("unique-code")
       if err != nil {
           fmt.Println(err)
           return
       }
       fmt.Printf("property value:: %v\n", val)
   }
   ```

9. Add metadata to an object

   ```go
   package main

   import (
       _ "oss.nandlabs.io/golly-gcp/storage"
       "oss.nandlabs.io/golly/vfs"
   )

   func main() {
       manager := vfs.GetManager()
       u, err := url.Parse("gs://{bucket_name}/folder_1/gopher-image.png")
       if err != nil {
           fmt.Println(err)
           return
       }
       file, err := manager.Open(u)
       if err != nil {
           fmt.Println(err)
           return
       }
       err = file.AddProperty("unique-code", "golly-image")
       if err != nil {
           fmt.Println(err)
           return
       }
   }
   ```

## Contributing

We welcome contributions to the Storage library! If you find a bug, have a feature request, or want to contribute improvements, please create a pull request. For major changes, please open an issue first to discuss the changes you would like to make.
