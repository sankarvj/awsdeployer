steps 

* You can compile and upload a revision by running `bash createRevision.sh`
  * `createRevision.sh` uses the following env var
    * `VERSION`: version numbe for which is for (make sure to include alpha0/beta1 if staging) (Required)
    * `PROJHOME`: `deepak-bala\konotor` repo's root directory (Optional)
    * `AWSPROFILE`: `aws cli`'s profile to use (Optional)
* goto the UI and hit the deploy button

Eg: 
```
VERSION="9.15.4" bash createrevision.sh
VERSION="9.15.4" AWSPROFILE="hlst" bash createrevision.sh
PROJHOME="/Users/user/sandbox/hotline-frontend" VERSION="9.15.4" bash -x createrevision.sh
```
