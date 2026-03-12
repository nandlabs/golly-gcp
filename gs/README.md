# gs

Google Cloud Storage implementation of the [golly VFS](https://pkg.go.dev/oss.nandlabs.io/golly/vfs) (Virtual File System) interface.

---

- [Installation](#installation)
- [Features](#features)
- [Architecture](#architecture)
- [Auto-Registration](#auto-registration)
- [URL Format](#url-format)
- [Configuration](#configuration)
  - [How It Works](#how-it-works)
  - [Config Options](#config-options)
  - [Setup Examples](#setup-examples)
- [Usage](#usage)
- [Error Handling](#error-handling)
- [API Reference](#api-reference)
- [Prerequisites](#prerequisites)
- [Contributing](#contributing)

---

## Installation

```bash
go get oss.nandlabs.io/golly-gcp/gs
```

## Features

### File Operations

- **Read** — stream object content from GCS
- **Write** — buffered writes flushed to GCS on `Close()`
- **Delete** — delete a single object
- **DeleteAll** — recursively delete all objects under a prefix
- **ListAll** — list all objects under a prefix
- **Info** — get object metadata (size, last modified, content type, directory check)
- **Parent** — navigate to the parent prefix
- **AddProperty / GetProperty** — read and write custom GCS object metadata
- **ContentType** — retrieve the MIME type of the object

### File System Operations

- **Create** — create a new empty object
- **Open** — open an existing object for reading/writing
- **Mkdir / MkdirAll** — create directory markers (zero-byte objects with trailing `/`)
- **Copy** — server-side copy using GCS `CopierFrom`
- **Move** — copy + delete
- **Delete** — delete object or recursively delete prefix
- **List** — list direct children of a prefix (files and common prefixes)
- **Walk** — recursively traverse all objects under a prefix
- **Find** — filter objects using a custom `FileFilter` function
- **DeleteMatching** — delete objects matching a filter

All operations also have `*Raw` variants that accept URL strings instead of `*url.URL`.

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│  Application                                                     │
│                                                                  │
│  import _ "oss.nandlabs.io/golly-gcp/gs"                         │
│                                                                  │
│  mgr := vfs.GetManager()                                         │
│  mgr.OpenRaw("gs://bucket/key")                                  │
│  mgr.CreateRaw("gs://bucket/key")                                │
│  mgr.CopyRaw(src, dst)                                           │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│  golly/vfs.Manager                                               │
│                                                                  │
│  Routes to filesystem by URL scheme ("gs")                       │
│  Calls StorageFS.Open / Create / Copy / List / Walk / Delete ... │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│  gs.StorageFS (VFileSystem)                                      │
│                                                                  │
│  1. parseURL(u)              → bucket + key from "gs://b/k"      │
│  2. getStorageClient(opts)   → gcpsvc.GetConfig(u, "gs")         │
│  3. GCS API call             → storage.Client operations         │
│  4. Returns StorageFile      → implements VFile (Read/Write/Close)│
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│  gcpsvc.Manager                                                  │
│                                                                  │
│  Config resolution chain:                                        │
│  url.Host → url.Host+"/"+url.Path → fallback name ("gs")         │
│                                                                  │
│  Returns *gcpsvc.Config → []option.ClientOption                  │
└──────────────────────────────────────────────────────────────────┘
```

## Auto-Registration

On package import, the `init()` function in `pkg.go` creates a `StorageFS` and registers it with `vfs.GetManager()`:

```go
func init() {
    storageFs := &StorageFS{}
    storageFs.BaseVFS = &vfs.BaseVFS{VFileSystem: storageFs}
    vfs.GetManager().Register(storageFs)
}
```

This means a **blank import** is all you need to make the GCS filesystem available:

```go
import _ "oss.nandlabs.io/golly-gcp/gs"
```

After this import, any call to `vfs.GetManager().OpenRaw("gs://...")` will automatically route to this filesystem.

## URL Format

```
gs://bucket-name/path/to/object.txt
gs://bucket-name/path/to/folder/
```

| Component | Maps To                        |
| --------- | ------------------------------ |
| Scheme    | `gs`                           |
| Host      | GCS bucket name                |
| Path      | Object key (prefix + filename) |

**Examples:**

| URL                                   | Bucket          | Key                     | Type      |
| ------------------------------------- | --------------- | ----------------------- | --------- |
| `gs://my-bucket/data/report.csv`      | `my-bucket`     | `data/report.csv`       | File      |
| `gs://my-bucket/logs/`                | `my-bucket`     | `logs/`                 | Directory |
| `gs://my-bucket/archive/2026/jan.zip` | `my-bucket`     | `archive/2026/jan.zip`  | File      |
| `gs://backup-bucket/`                 | `backup-bucket` | _(empty — bucket root)_ | Directory |

## Configuration

gs uses the [`gcpsvc`](../gcpsvc/) package for GCP configuration management. At the core of this system is `gcpsvc.Manager` — a named registry of `*gcpsvc.Config` instances. You register configs under keys, and gs automatically resolves the right config for each GCS URL.

### How It Works

#### 1. Registration

Before performing any GCS operations, you register one or more `*gcpsvc.Config` instances with `gcpsvc.Manager`:

```go
cfg := &gcpsvc.Config{
    ProjectId: "my-project",
    Location:  "us-central1",
}
cfg.SetAuthCredentialFile(option.ServiceAccount, "/path/to/credentials.json")
gcpsvc.Manager.Register("gs", cfg)
```

`gcpsvc.Manager` is a `managers.ItemManager[*Config]` — a typed, thread-safe, named key-value store. You can register any number of configs under different keys:

```go
gcpsvc.Manager.Register("gs", defaultCfg)              // fallback for all GCS ops
gcpsvc.Manager.Register("my-bucket", bucketSpecificCfg) // bucket-specific
gcpsvc.Manager.Register("logs-bucket", logsCfg)         // another bucket
```

#### 2. Resolution

When gs needs a storage client (e.g., to read `gs://my-bucket/data/file.txt`), it calls:

```go
cfg := gcpsvc.GetConfig(parsedURL, "gs")
```

`GetConfig` resolves the config using a **three-step fallback chain**:

| Step | Lookup Key                  | Example for `gs://my-bucket/data/file.txt` | Purpose                        |
| ---- | --------------------------- | ------------------------------------------ | ------------------------------ |
| 1    | `url.Host`                  | `Manager.Get("my-bucket")`                 | Bucket-specific config         |
| 2    | `url.Host + "/" + url.Path` | `Manager.Get("my-bucket/data/file.txt")`   | Path-specific config           |
| 3    | Fallback name (`"gs"`)      | `Manager.Get("gs")`                        | Default config for all GCS ops |

The first non-nil result is used. If all three return nil, gs creates a default storage client using Application Default Credentials (ADC).

#### 3. Client Creation

Once a config is resolved, gs creates a `storage.Client` using the config's client options:

```go
storage.NewClient(ctx, cfg.Options...)
```

If no config is registered, it falls back to:

```go
storage.NewClient(ctx) // uses Application Default Credentials
```

```
┌──────────────────┐     ┌────────────────────────┐     ┌───────────────────┐
│  GCS URL         │────▶│  gcpsvc.GetConfig()    │────▶│  *gcpsvc.Config   │
│  gs://bucket/key │     │  (3-step resolution)   │     │  (Options, Proj)  │
└──────────────────┘     └────────────────────────┘     └────────┬──────────┘
                                                                 │
                                                                 ▼
                                                        ┌──────────────────┐
                                                        │  storage.Client  │
                                                        │  (ready to use)  │
                                                        └──────────────────┘
```

### Config Options

`gcpsvc.Config` supports the following fields and setters:

| Field / Setter                          | Description                                 |
| --------------------------------------- | ------------------------------------------- |
| `ProjectId` / `SetProjectId(id)`        | GCP project ID                              |
| `Location` / `SetRegion(region)`        | GCP region/location (e.g., `"us-central1"`) |
| `SetAuthCredentialFile(credType, path)` | Service account JSON key file               |
| `SetAuthCredentialJSON(credType, json)` | Service account JSON key as byte slice      |
| `SetEndpoint(url)`                      | Custom endpoint URL (for emulators, etc.)   |
| `SetUserAgent(ua)`                      | Custom user agent string                    |
| `SetQuotaProject(project)`              | Quota project for billing                   |
| `SetScopes(scopes...)`                  | OAuth2 scopes                               |
| `AddOption(opt)`                        | Append any custom `option.ClientOption`     |

### Setup Examples

#### Basic Setup

```go
package main

import (
    "oss.nandlabs.io/golly-gcp/gcpsvc"
    _ "oss.nandlabs.io/golly-gcp/gs" // auto-registers with VFS manager
    "oss.nandlabs.io/golly/vfs"
)

func main() {
    // Register a default GCP config for all GCS operations
    cfg := &gcpsvc.Config{
        ProjectId: "my-project",
        Location:  "us-central1",
    }
    cfg.SetAuthCredentialFile(option.ServiceAccount, "/path/to/credentials.json")
    gcpsvc.Manager.Register("gs", cfg)

    // Now use the VFS manager — gs resolves config automatically
    mgr := vfs.GetManager()
    file, _ := mgr.OpenRaw("gs://my-bucket/data/file.txt")
    // ...
}
```

#### With Service Account JSON Key

```go
cfg := &gcpsvc.Config{ProjectId: "my-project"}
cfg.SetAuthCredentialFile(option.ServiceAccount, "/path/to/service-account.json")
gcpsvc.Manager.Register("gs", cfg)
```

#### With Inline Credentials JSON

```go
cfg := &gcpsvc.Config{ProjectId: "my-project"}
cfg.SetCredentialJSON([]byte(`{"type": "service_account", ...}`))
gcpsvc.Manager.Register("gs", cfg)
```

#### With Application Default Credentials (ADC)

If you don't register any config, or register a config without credentials, the Google Cloud SDK will use [Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials):

```go
// No explicit config needed — uses ADC from environment
// (gcloud auth, GOOGLE_APPLICATION_CREDENTIALS, metadata server, etc.)
mgr := vfs.GetManager()
file, _ := mgr.OpenRaw("gs://my-bucket/data/file.txt")
```

#### With Custom Endpoint (Emulator)

```go
cfg := &gcpsvc.Config{ProjectId: "test-project"}
cfg.SetEndpoint("http://localhost:4443")
gcpsvc.Manager.Register("gs", cfg)
```

#### Per-Bucket Configuration

Register different configs for different buckets. The bucket name in the GCS URL is matched against the registration key:

```go
// Default fallback for any bucket without a specific config
defaultCfg := &gcpsvc.Config{ProjectId: "my-project"}
defaultCfg.SetAuthCredentialFile(option.ServiceAccount, "/path/to/default-creds.json")
gcpsvc.Manager.Register("gs", defaultCfg)

// Bucket-specific: production data with prod credentials
prodCfg := &gcpsvc.Config{ProjectId: "prod-project"}
prodCfg.SetAuthCredentialFile(option.ServiceAccount, "/path/to/prod-creds.json")
gcpsvc.Manager.Register("prod-data-bucket", prodCfg)

// Bucket-specific: EU data in a different project
euCfg := &gcpsvc.Config{ProjectId: "eu-project", Location: "europe-west1"}
euCfg.SetAuthCredentialFile(option.ServiceAccount, "/path/to/eu-creds.json")
gcpsvc.Manager.Register("eu-data-bucket", euCfg)
```

With the above registration:

| GCS URL                                | Config Resolved | Project      | Why                                      |
| -------------------------------------- | --------------- | ------------ | ---------------------------------------- |
| `gs://prod-data-bucket/reports/q1.csv` | `prodCfg`       | prod-project | Host `"prod-data-bucket"` matches step 1 |
| `gs://eu-data-bucket/logs/app.log`     | `euCfg`         | eu-project   | Host `"eu-data-bucket"` matches step 1   |
| `gs://any-other-bucket/data.json`      | `defaultCfg`    | my-project   | No host match → falls back to `"gs"`     |

## Usage

### Reading a File

```go
file, err := vfs.GetManager().OpenRaw("gs://my-bucket/data/report.csv")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

content, err := file.AsString()
if err != nil {
    log.Fatal(err)
}
fmt.Println(content)
```

### Writing a File

```go
file, err := vfs.GetManager().CreateRaw("gs://my-bucket/output/result.json")
if err != nil {
    log.Fatal(err)
}

_, err = file.WriteString(`{"status": "ok"}`)
if err != nil {
    log.Fatal(err)
}
// Data is flushed to GCS on Close
err = file.Close()
```

### Listing Files

```go
files, err := vfs.GetManager().ListRaw("gs://my-bucket/data/")
if err != nil {
    log.Fatal(err)
}
for _, f := range files {
    info, _ := f.Info()
    fmt.Printf("%s (size: %d, dir: %t)\n", info.Name(), info.Size(), info.IsDir())
}
```

### Walking a Directory Tree

```go
err := vfs.GetManager().WalkRaw("gs://my-bucket/logs/", func(file vfs.VFile) error {
    info, _ := file.Info()
    fmt.Println(info.Name())
    return nil
})
```

### Copying Files

```go
// Server-side copy within GCS
err := vfs.GetManager().CopyRaw(
    "gs://src-bucket/data/file.txt",
    "gs://dst-bucket/backup/file.txt",
)
```

### Moving Files

```go
err := vfs.GetManager().MoveRaw(
    "gs://my-bucket/temp/upload.dat",
    "gs://my-bucket/archive/upload.dat",
)
```

### Creating Directories

```go
dir, err := vfs.GetManager().MkdirRaw("gs://my-bucket/new-folder/")
if err != nil {
    log.Fatal(err)
}
defer dir.Close()
```

### Deleting Files

```go
// Delete a single file
err := vfs.GetManager().DeleteRaw("gs://my-bucket/old-file.txt")

// Delete a directory and all its contents
err = vfs.GetManager().DeleteRaw("gs://my-bucket/old-folder/")
```

### Working with Metadata

```go
file, _ := vfs.GetManager().OpenRaw("gs://my-bucket/data/report.csv")
defer file.Close()

// Add metadata
file.AddProperty("department", "engineering")

// Read metadata
dept, _ := file.GetProperty("department")
fmt.Println(dept) // "engineering"
```

### Finding Files with a Filter

```go
location, _ := url.Parse("gs://my-bucket/data/")
csvFiles, err := vfs.GetManager().Find(location, func(file vfs.VFile) (bool, error) {
    info, err := file.Info()
    if err != nil {
        return false, err
    }
    return strings.HasSuffix(info.Name(), ".csv"), nil
})
```

## API Reference

### StorageFS (VFileSystem)

| Method                      | Description                                 |
| --------------------------- | ------------------------------------------- |
| `Schemes()`                 | Returns `["gs"]`                            |
| `Create(u)`                 | Creates a new empty GCS object              |
| `Open(u)`                   | Opens a GCS object (lazy — no network call) |
| `Mkdir(u)` / `MkdirAll(u)`  | Creates a directory marker                  |
| `Copy(src, dst)`            | Server-side copy using `CopierFrom`         |
| `Move(src, dst)`            | Copy + delete                               |
| `Delete(src)`               | Delete object or recursive prefix delete    |
| `List(u)`                   | List direct children (with delimiter)       |
| `Walk(u, fn)`               | Recursive traversal of all objects          |
| `Find(u, filter)`           | Find objects matching a filter              |
| `DeleteMatching(u, filter)` | Delete objects matching a filter            |

### StorageFile (VFile)

| Method                 | Description                                       |
| ---------------------- | ------------------------------------------------- |
| `Read(b)`              | Streams object content from GCS                   |
| `Write(b)`             | Buffers data (flushed on Close)                   |
| `Seek(offset, whence)` | Reset to start only (`SeekStart`, 0)              |
| `Close()`              | Flushes writes to GCS, closes readers             |
| `ListAll()`            | Lists all objects under this prefix               |
| `Delete()`             | Deletes this object                               |
| `DeleteAll()`          | Recursively deletes all objects under this prefix |
| `Info()`               | Returns `StorageFileInfo`                         |
| `Parent()`             | Returns parent prefix as `VFile`                  |
| `Url()`                | Returns the GCS URL                               |
| `ContentType()`        | Returns the MIME content type                     |
| `AddProperty(k, v)`    | Sets GCS custom metadata                          |
| `GetProperty(k)`       | Gets GCS custom metadata                          |
| `AsString()`           | Reads entire content as string                    |
| `AsBytes()`            | Reads entire content as byte slice                |
| `WriteString(s)`       | Writes a string to the buffer                     |

### StorageFileInfo (VFileInfo)

| Method      | Description                 |
| ----------- | --------------------------- |
| `Name()`    | Object key                  |
| `Size()`    | Size in bytes               |
| `Mode()`    | Always `0` (not applicable) |
| `ModTime()` | Last modified time          |
| `IsDir()`   | `true` if prefix/directory  |
| `Sys()`     | Returns the `VFileSystem`   |

## Error Handling

### URL Validation Errors

All operations validate the GCS URL before making any API calls:

| Error                                             | When                                |
| ------------------------------------------------- | ----------------------------------- |
| `url cannot be nil`                               | A nil `*url.URL` was passed         |
| `invalid URL scheme, expected 'gs'`               | URL scheme is not `gs`              |
| `invalid GCS URL, bucket name (host) is required` | URL has no host (e.g., `gs:///key`) |

### File System Errors

| Error                                     | When                                                  |
| ----------------------------------------- | ----------------------------------------------------- |
| `file gs://bucket/key already exists`     | `Create` called for an object that already exists     |
| `seek not fully supported on GCS objects` | `Seek` called with anything other than `SeekStart, 0` |
| `failed to get object metadata: ...`      | `AddProperty` / `GetProperty` — object attrs failed   |
| `failed to update object metadata: ...`   | `AddProperty` — metadata update failed                |
| `metadata key "..." not found`            | `GetProperty` — requested key not in custom metadata  |

### GCS API Errors

All GCS API calls can return Google Cloud errors. Common examples:

| Error                             | Typical Cause                                     |
| --------------------------------- | ------------------------------------------------- |
| `storage: bucket doesn't exist`   | The bucket does not exist                         |
| `storage: object doesn't exist`   | The object does not exist                         |
| `googleapi: Error 403: Forbidden` | IAM policy does not grant the required permission |
| `googleapi: Error 404: Not Found` | Resource not found                                |
| `googleapi: Error 409: Conflict`  | Precondition failed (e.g., generation mismatch)   |
| `context deadline exceeded`       | Network timeout or slow connection                |

### Write Behavior

Writes are **buffered in memory** and only flushed to GCS when `Close()` is called. If `Close()` returns an error, the data was **not** persisted to GCS. Always check the error from `Close()`:

```go
file, _ := vfs.GetManager().CreateRaw("gs://bucket/key")
file.WriteString("data")

// IMPORTANT: check the error — this is where the upload happens
if err := file.Close(); err != nil {
    log.Fatalf("failed to write to GCS: %v", err)
}
```

### Copy Behavior

`Copy` uses GCS **server-side copy** via `CopierFrom`, which copies objects without transferring data through your application. This works across buckets within the same project. Directory copies recursively copy all children.

### Directory Semantics

GCS has no native directory concept. This package simulates directories using:

- **Trailing slash keys**: `data/` is a zero-byte object acting as a directory marker
- **Common prefixes**: `Objects()` with a delimiter groups keys by prefix
- **Prefix detection**: If object attrs fail but listing with `prefix + "/"` returns results, the path is treated as a directory

Operations like `Delete`, `Walk`, and `ListAll` automatically handle recursive prefix traversal.

## Prerequisites

### GCS Permissions

The IAM principal used must have the following GCS permissions depending on the operations performed:

| Permission                       | Required For                                                             |
| -------------------------------- | ------------------------------------------------------------------------ |
| `storage.objects.get`            | `Read`, `Open` (when reading), `AsString`, `AsBytes`, `Info`             |
| `storage.objects.create`         | `Create`, `Write`, `Close` (flush), `Mkdir`, `MkdirAll`, `Copy`          |
| `storage.objects.delete`         | `Delete`, `DeleteAll`, `DeleteMatching`, `Move`                          |
| `storage.objects.list`           | `List`, `Walk`, `Find`, `ListAll`, `DeleteAll`, `Info` (directory check) |
| `storage.objects.getMetadata`    | `Info`, `Create` (existence check), `AddProperty`, `GetProperty`         |
| `storage.objects.updateMetadata` | `AddProperty`                                                            |

**Minimal predefined role for read-only access:**

- `roles/storage.objectViewer` — grants `storage.objects.get` and `storage.objects.list`

**Full access predefined role:**

- `roles/storage.objectAdmin` — grants all object-level permissions

**Custom IAM policy binding example:**

```bash
gcloud projects add-iam-policy-binding my-project \
    --member="serviceAccount:my-sa@my-project.iam.gserviceaccount.com" \
    --role="roles/storage.objectAdmin"
```

### GCP Authentication

Credentials can be provided through any of the following methods:

- `gcpsvc.Config` with `SetAuthCredentialFile(credType, path)` — service account JSON key file
- `gcpsvc.Config` with `SetAuthCredentialJSON(credType, json)` — inline service account JSON
- `GOOGLE_APPLICATION_CREDENTIALS` environment variable
- Application Default Credentials (`gcloud auth application-default login`)
- GCE instance metadata (when running on Google Cloud)
- GKE Workload Identity
- Cloud Run / Cloud Functions default service account

## Contributing

We welcome contributions. If you find a bug or would like to request a new feature, please open an issue on [GitHub](https://github.com/nandlabs/golly-gcp/issues).

````

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
````

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

6. List all the files in a bucket

   ```go
   package main

   import (
       _ "oss.nandlabs.io/golly-gcp/storage"
       "oss.nandlabs.io/golly/vfs"
   )

   func main() {
       manager := vfs.GetManager()
       u, err := url.Parse("gs://{bucket_name}")
       if err != nil {
           fmt.Println(err)
           return
       }
       file, err := manager.Open(u)
       if err != nil {
           fmt.Println(err)
           return
       }
       files, err := file.ListAll()
       if err != nil {
           fmt.Println(err)
           return
       }
       fmt.Print(files)
   }
   ```

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
