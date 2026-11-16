# airport-db-client

[![Lint](https://github.com/aviation-tragic-au/airport-db-client/actions/workflows/lint.yml/badge.svg)](https://github.com/aviation-tragic-au/airport-db-client/actions/workflows/lint.yml) [![Test](https://github.com/aviation-tragic-au/airport-db-client/actions/workflows/test.yml/badge.svg)](https://github.com/aviation-tragic-au/airport-db-client/actions/workflows/test.yml)

Provides a client for the AirportDB API.

https://airportdb.io/

## Install

```sh
go get github.com/aviation-tragic-au/airport-db-client
```

## Usage

```go
import (
    "context"
    "errors"
    "log"

    airportdb "github.com/aviation-tragic-au/airport-db-client"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewProduction()

    // URL defaults to https://airportdb.io/api/v1/airport. Set KeyFunc
    // instead of Key to look the key up on every request (to pick up a key
    // changed at runtime), and HTTPClient to use your own transport or
    // timeout (the default times out after 10 seconds).
    client, err := airportdb.NewClient(logger, airportdb.Config{
        Key: "your-api-token",
    })
    if err != nil {
        log.Fatal(err)
    }

    airport, err := client.GetAirport(context.Background(), "YBBN")
    switch {
    case errors.Is(err, airportdb.ErrNotFound):
        log.Print("AirportDB doesn't have that airport")
    case err != nil:
        log.Fatal(err)
    default:
        log.Printf("Airport: %s (%s)", airport.Name, airport.ICAOCode)
    }
}
```

`GetAirport` returns `ErrNotFound` for a 404, and for the empty record
AirportDB sometimes answers with instead. Any other non-200 response
(a rate-limited 429, say) is an error carrying the status and the start of
the body.

## Contributing

1. Fork it
2. Create your feature branch (`git checkout -b my-new-feature`)
3. Commit your changes (`git commit -am 'Add some feature'`)
4. Push to the branch (`git push origin my-new-feature`)
5. Create a new Pull Request

## License

Copyright 2026 Dean Stalker

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.