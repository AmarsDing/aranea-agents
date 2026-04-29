// Package cron is Operations' driving Cron adapter: it ticks scheduled jobs
// and dispatches them to operations/application/cron_service. It is the
// only place permitted to depend on a concrete cron library.
package cron
