// Package config to parse ~/.dscli/dscli.env or ~/.dscli/config.dscli
//
// # Dscli config format
//
// For example
//
//  # Dscli config format
//  key1 = val1
//  key2 = val2 # comment
//  # compatible with env
//  key3=val3
//  export key4=val4
//
// # Load order and save
//
//  1. If config.dscli there, load and return
//  2. Or load dscli.env and save to config.dscli
//  3. Or load from env and save to config.dscli

package config
