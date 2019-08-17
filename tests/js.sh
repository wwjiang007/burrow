#!/usr/bin/env bash
# ----------------------------------------------------------
# PURPOSE

# This is the test manager for playbooks. It will run the testing
# sequence for playbooks referencing test fixtures in this tests directory.

# ----------------------------------------------------------
# REQUIREMENTS

# m

# ----------------------------------------------------------
# USAGE

# run_pkgs_tests.sh [appXX]


export script_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "$script_dir/test_runner.sh"

export js_dir="${script_dir}/../js"

perform_js_tests(){
  cd "$js_dir"
  test_account="{\"address\": \"$key1_addr\"}"
  echo "Using test account:"
  account="$test_account" mocha --bail --exit --recursive ${1}
  test_exit=$?
}

burrowjs_tests() {
    echo "Hello! I'm the marmot that tests burrow-js."
    echo

    test_setup
    trap test_teardown EXIT

    echo "Running js tests..."
    perform_js_tests "$1"
}

burrowjs_tests "$1"
