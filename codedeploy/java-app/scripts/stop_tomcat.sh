#!/bin/bash

sudo systemctl stop tomcat.service
rm -rf /opt/tomcat/webapps/ROOT/
rm -rf /opt/tomcat/webapps/ROOT.war
