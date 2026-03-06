package com.example;

import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import java.util.List;
import java.util.Optional;

@Path("/users")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
public class UserController {

    @GET
    @Path("/")
    public List<User> getUsers() {
        return null;
    }

    @GET
    @Path("/{id}")
    public User getUserById(@PathParam("id") Long id) {
        return null;
    }

    @POST
    @Path("/")
    public Response createUser(User user) {
        return Response.ok(user).build();
    }

    @PUT
    @Path("/{id}")
    public Response updateUser(@PathParam("id") Long id, User user) {
        return Response.ok(user).build();
    }

    @DELETE
    @Path("/{id}")
    public Response deleteUser(@PathParam("id") Long id) {
        return Response.noContent().build();
    }

    @GET
    @Path("/{id}/orders")
    public Response getUserOrders(@PathParam("id") Long id) {
        List<Order> orders = findOrdersByUserId(id);
        return Response.ok(orders).build();
    }

    @POST
    @Path("/{id}/orders")
    public Response createOrder(@PathParam("id") Long id, Order order) {
        if (id == null) {
            return Response.status(400).build();
        }
        Order created = saveOrder(id, order);
        return Response.status(201).entity(created).build();
    }

    private List<Order> findOrdersByUserId(Long userId) {
        return null;
    }

    private Order saveOrder(Long userId, Order order) {
        return order;
    }
}
