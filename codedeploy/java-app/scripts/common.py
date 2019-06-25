import urllib2
import boto3
import json
import os
import time

INSTANCE_DETAILS = json.loads(urllib2.urlopen("http://instance-data/latest/dynamic/instance-identity/document").read())
DEPLOYMENT_GROUP_ID = os.environ['DEPLOYMENT_GROUP_ID']
JSON_ELBS_FILE = "/tmp/deploy_"+DEPLOYMENT_GROUP_ID+"_elbs.json"

session = boto3.Session(region_name=INSTANCE_DETAILS['region'])
elb_client = session.client("elb")

def getElbsAssociated():
	elbs = elb_client.describe_load_balancers()
	associated_elbs = []
	for elb in elbs['LoadBalancerDescriptions']:
		elb_name = elb['LoadBalancerName']
		for instance in elb['Instances']:
			if INSTANCE_DETAILS['instanceId'] == instance['InstanceId']:
				associated_elbs.append(elb['LoadBalancerName'])
				continue
	with open(JSON_ELBS_FILE, 'w') as ofile:
		json.dump(associated_elbs, ofile)

def getElbs():
	with open(JSON_ELBS_FILE) as elbs_json:    
		return json.load(elbs_json)

def deregisterThisFromElbs():
	instances = [{'InstanceId': INSTANCE_DETAILS['instanceId']}]
	for elb_name in getElbs():
		resp = elb_client.deregister_instances_from_load_balancer(LoadBalancerName=elb_name, Instances=instances)
		print "deregistered instance from elb, instances: " + str(instances) + ", elb: " + elb_name + "; resp: " + str(resp)

def registerThisWithElbs():
	instances = [{'InstanceId': INSTANCE_DETAILS['instanceId']}]
	for elb_name in getElbs():
		resp = elb_client.register_instances_with_load_balancer(LoadBalancerName=elb_name, Instances=instances)
		print "registered instance from elb, instances: " + str(instances) + ", elb: " + elb_name + "; resp: " + str(resp)

def checkInstanceElbState():
	instances = [{'InstanceId': INSTANCE_DETAILS['instanceId']}]
	for elb_name in getElbs():
		recheck = True
		checkNum = 0
		checkThreshold = 10
		instanceState = 'UNCHECKED'
		while recheck and checkNum < checkThreshold:
			resp = elb_client.describe_instance_health(LoadBalancerName=elb_name, Instances=instances)
			instanceState = resp['InstanceStates'][0]['State']
			if instanceState != 'InService':
				print "[WARN] instance " + INSTANCE_DETAILS['instanceId'] + " not in service yet, retrying"
				time.sleep(4)
				checkNum = checkNum+1
			else:
				recheck = False
		if instanceState != 'InService':
			print "[FATAL] instance " +  INSTANCE_DETAILS['instanceId'] + " failed to register with elb, instances: " + str(instances) + ", elb: " + elb_name 
			exit(1)
		else:
			print "[OK] instance " + INSTANCE_DETAILS['instanceId'] + ", is now in service"
