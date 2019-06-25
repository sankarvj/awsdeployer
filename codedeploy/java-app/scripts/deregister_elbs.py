#!/usr/bin/python
import common
import time

common.deregisterThisFromElbs()
# wait for connections to drain
time.sleep(3)
