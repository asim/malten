# Malten

Anonymous ephmeral messaging

## Overview

Malten is designed as a secure anonymous ephemeral messaging service. It contains solely streams of text and nothing else. 
Streams are ephemeral, with a lifetime of 24 hours. Each stream supports 1000 messages and 512 characters per message. There 
can only ever be 1000 streams at any given time. Streams can be discovered through exploration or by listing them via the API.

## Usage

```
go get github.com/asim/malten && malten
```

## API

To retrieve a list of streams

```
GET /streams
```

To retrieve thoughts

```
GET /thoughts
```

Params for thoughts

```
stream: string e.g stream=foo
limit: integer e.g limit=25
direction: integer e.g direction=1 or direction=-1
last: integer e.g last=1652035113591000000
```

An example retrieval

```
GET /thoughts?direction=1&limit=25&last=1652035113591000000&stream=foo
```

To create thoughts

```
POST /thoughts
```

Params for thoughts

```
stream: string e.g stream=foo
text: string e.g text=helloworld
```

An example

```
POST /thoughts?stream=foo&text=helloworld
```

## Web

Go to https://malten.com

## Testing 

For development http://localhost:9090
