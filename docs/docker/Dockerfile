FROM nginx:alpine

# Remove default config
RUN rm /etc/nginx/conf.d/default.conf

# Add SPA-aware nginx config
COPY nginx.conf /etc/nginx/conf.d/

# App files go in /usr/share/nginx/html
COPY dist/ /usr/share/nginx/html/

EXPOSE 8080
