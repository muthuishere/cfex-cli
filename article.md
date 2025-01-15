### Cftun

I have built a small wrapper around the Cloudflare Tunnel client  to make it easier to use , like a single command to expose your local app to internet with your preferred domain name.

Say in your machine if its running a web server on port 8080 and you want to expose it to the internet with a domain name `myapp.com` , you can use the below command to do that.

```bash
cfex myapp.com 8080
```

Now you can access your local app from anywhere in the world using the domain name `https://myapp.com` with ssl enabled.


### How it works

Cloudflare provides tunnel to your machine through our cfex app , it creates a tunnel between your machine and cloudflare edge network and routes the traffic to your local machine.

### When it would be useful

Most of the development environment does require a seperate server , seperate ci/cd pipeline and expose it and then you handover the dev to product owner or qa team to test it. If there is an issue , you need to again go back to the code , fix it and then again deploy it , its a time cost.

With the help of cfex , you can expose your local app to the internet with a custom domain name  and multiple teams can have their dev environment exposed to the internet with their own domain name and you can use it for testing as well. We dont want to unnecessarily deploy the code to the server and then test it. And this might not be possible for all the projects , but for some projects where you can use this , it will be very helpful.



This will be helpful ,  you do the dev activity , expose to internet , share the link to the product owner or qa team to test it. If there is an issue , you fix it then and there, no need to deploy it again and again.






So what do you need to do to get this working?

- A cloudflare account it is free
- A domain name that you own and is managed by cloudflare dns (Buy one from cloudflare or transfer your existing domain to cloudflare , its easy and free)
- Install the cloudflare tunnel client in your machine , you can download it from the cloudflare dashboard
- Install jq in your machine , you can install it using `brew install jq` or `sudo apt-get install jq` or `sudo yum install jq` based on your os
- Get a cloudflare api token with zone permissions , you can create one from the cloudflare dashboard

Once you have all the above setup you can start using the `cfex` command to expose your local apps to the internet.


### Installation
