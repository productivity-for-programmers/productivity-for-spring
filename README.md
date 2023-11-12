# productivity-for-spring
Local development web server for Spring Boot that recompiles upon source file changes and pauses 
connections while the Spring Boot project is recompiling/restarting. This ensures that:
- clients never get an outdated response
- clients never get a connection refused error
 
## Usage

```
go run main.go --help
Usage of /tmp/go-build778935637/b001/exe/main:
  -base-url string
    	Base URL (default "http://localhost:8080")
  -build-command string
    	Build command (default "./gradlew build -x test")
  -health-check-path string
    	Health Check Endpoint (default "/actuator/health")
  -spring-dir string
    	Directory of the Spring Boot project (default ".")
```

## Health Check

A health check endpoint is required so that the web server knows when the Spring Boot application is ready to 
accept connections. The easiest way to get this endpoint is to use spring-boot-starter-actuator:

```
implementation 'org.springframework.boot:spring-boot-starter-actuator'
```

## Sample Usage

https://www.youtube.com/watch?v=0NwhHmFbJPk

## Limitations

- Hardcoded to run on port 9000
- Hardcoded to use bash to run the build command 
- The rebuild needs to be triggered from this web server itself so that it knows exactly when the build process
  finishes and how long to wait before spring-boot-devtools will catch the class file changes and trigger a restart
- Only listens to changes to .java files
- Tested with Go 1.21.4
- Tested with Spring Boot 3.1.4
- Only tested on Linux