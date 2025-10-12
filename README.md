# Load Balancer

[![Build](https://img.shields.io/github/actions/workflow/status/JakeRoggenbuck/load-balancer/go.yml?branch=main&style=for-the-badge)](https://github.com/JakeRoggenbuck/load-balancer/actions)
[![Go](https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://github.com/JakeRoggenbuck?tab=repositories&q=&type=&language=go&sort=stargazers)
[![Docker](https://img.shields.io/badge/Docker-2CA5E0?style=for-the-badge&logo=docker&logoColor=white)](#)

Load balancer for HTTP requests using round robin algorithm written in Go.

### Demo

<img width="1896" height="1027" alt="2025-10-12_13-44" src="https://github.com/user-attachments/assets/54f67497-d368-41b8-b1fe-088a5003f170" />

The load balancer is responding to requests and routing them to two different FastAPI servers and then returning their responses.

### Config

The load-balancer uses a TOML config to define infrastructure.

The values are `alive` which tells the load-balancer if the application is alive or not. You can default it to alive or dead on startup. You can set the `ip` of the application as well as the `port` as you'd expect. The `tsl` option is to help with local testing. This load balancer does not care if you use tls or not, so you can still use it for local host apps that do not have HTTPS yet.

This is all written to `applications.toml` in the directory that the load balancer is running.

```toml
[[applications]]
alive = true
ip = "127.0.0.1"
port = "8001"
tsl = false

[[applications]]
alive = true
ip = "127.0.0.1"
port = "8000"
tsl = false
```

### Running

You can use docker to run this load balancer with the following commands:

```sh
# Build the image
docker build -t load-balancer:latest .

# Run the image
docker run -p 8080:8080 load-balancer:latest
```

### Initial Ideas

I feel like you could either A. send a request to ask for what server to use or B. send all traffic to the load balancer, and the LB acts like a relay. For output, A. An IP address of the server to talk to or B. the result that was expected.

After thinking about it, to reduce load, you should have the load balancer send the client to a different IP. I feel like you could send a temporary redirect code or something and have the protocol handle some of the logic.

### Notes

I'm reading this white paper about load balancing: https://www.f5.com/pdf/white-papers/load-balancing101-wp.pdf

Term: Node or Server => The actual server that does the task, or the "physical server". The paper then calls it the "host".

Term: Member or Service (also sometimes called a Node) => This is the application running on the server than has a port associated with it that can be directed to.

We want to be able to talk to specific applications on physical servers instead of just talking to physical servers themselves.

Put simply, there is a "Host" that is a physical server which hosts applications or "Services" on it.

```
My Physical "Host"
[ (Application 1), (Application 2) ]
```

Term: Pool, Cluster, or Farm => a collection of "similar" or the same application running on a bunch of servers.

Basically, I want to be able to swap out any one application in the farm for another if I need to. Just another instance of the same app.

If I had an app that schedules meetings, I could send a schedule meeting request to app A or app B or app C and they would all get the job done. Say the "Host" of application A (a service) goes down (gets unplugged accidentally), I can just use app C or B instead and everything still works.

I may want to applications if one breaks unexpectedly, but writing this has made me think that the biggest reason is if you need to do planned maintenance on a server, you can just tell the load balancer that this is going to happen, and it will temporarily exclude the downed server from being an application you route to.

Term: Virtual Server => This refers to the system that does the load balancing.

After reading the process in "Load Balancing Basics", I does seem more similar to idea B, where everything does go through the virtual server as if it's the one sending all the data back.

Or actually, maybe the first request gets sent to the virtual server and then the destination IP is changed in the return packet so the next request goes directly to the host?

Okay, it seems like there is a "proxy mode" or "full NAT", where the load balancer acts like a proxy for all the traffic, and the client doesn't directly talk to the host. There is a mode called Direct Server Return where you can switch the IP and allow the host to talk directly with the client, but this is hard to do and might require special hardware.

For figuring out what service to send traffic to, you can do a bunch of things. Those include sending a ping and making sure the service is still online, or alternating which service you send to. You can also count the current amount of traffic, or anything more complex.

For my purposes, I feel like running a basic health check to see if the server is still online should be good enough to keep it in the pool. Then use a basic round robin approach, where you just give traffic to the next-in-line service. I'd probably use a list of pointers and an index and a modulo.

The article goes on to talk about "keep alive" and stuff like that, but that's not something I'm super interested in.

## Inputs

Send the load balancer your traffic. E.g. "/get-classes" as a route or maybe a URL parameter.

## Outputs

Send the response back the the clinet. E.g. a list of classes in JSON format.

## Design

You can have an internal array of the "Application" struct. You can then use an iterator to choose what server to go with for each subsequent request. I will be making this in Go, because its a good language to use for making fast, concurrent, web systems.

```go
type Application struct {
    Alive bool
    IP string
    Port int
}
```

And then you have a "Pool".

```go
type Pool struct {
    Apps []Application
}
```

You can then set a time interval to check the health of each server. You send a "/health" request to the server and if it's alive, set the state to alive.

You could remove an Application from the pool if it's not alive. You can have an alive pool and a dead pool move it from one to another.

It feels more simple though to just save the alive or dead state in the Application struct itself. Both options are interesting and valid though.

Also, if a request every fails because of a 500 or 400 error, you can just mark it as dead until it comes back alive if a later health check shows it's working.

A 500 from a specific route could mean that either the entire application is broken and it should be marked as dead, or the specific request had trouble executing. This gives some nuance for if you should mark a server dead or not from a 500 error. If the health check gives a 500, definitely mark it as dead.

Having a constant health check, you could also have a service that send an email or text for when an application is marked as dead. This can alert a team about an issue with one of the services.

I've had a server break in the middle of the night during a low traffic day and time of the year, and despite a couple dozen people visiting the site and experiencing issues, no one sent us an email. Often times if that application is down during the day, people send us a message (usually a text from a friend).
