# Malten

Streams of consciousness in the bicameral mind

## Overview

Malten is designed as an ephemeral stream of consciousness of the collective mind. 
It contains solely streams of text and nothing else. Streams are ephemeral, ever flowing and there are many 
of them. Streams can only be discovered through exploration but you may never actually find them unless 
you know what you're looking for.

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
