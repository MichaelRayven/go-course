package middleware_test

import "net/http"

const tripRoute = "/api/v1/trips/{tripId}"

func tripRouteFunc(*http.Request) string { return tripRoute }
