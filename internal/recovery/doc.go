// Package recovery implements metadata and data recovery for Backup Manager.
//
// The package is deliberately independent from the command line: it returns
// results and errors instead of printing or exiting. This keeps the recovery
// workflow testable with unit tests, fake repositories and temporary storage.
package recovery
